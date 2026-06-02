// Package repository 定义存证数据的访问层（Repository 模式）。
// 包含 CertifiedRepository（操作日志）和 ProofRepository（哈希链）两个接口。
package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// ============================================================
// 数据库行结构
// ============================================================

// CertifiedOpRow 对应 certified_operations 表的一行。
// db 标签用于 sqlx 的 StructScan / NamedExec 字段映射。
type CertifiedOpRow struct {
	ID        int64  `db:"id"`
	Timestamp int64  `db:"timestamp"`
	Type      string `db:"type"`
	UserName  string `db:"user_name"`
	UserUID   int    `db:"user_uid"`
	Action    string `db:"action"`
	Path      string `db:"path"`
	DestPath  string `db:"dest_path"`
	Detail    string `db:"detail"`
	FileName  string `db:"file_name"`
	IsDir     bool   `db:"is_dir"`
	FileSize  int64  `db:"file_size"`
	MimeType  string `db:"mime_type"`
	OwnerUID  int    `db:"owner_uid"`
	OwnerName string `db:"owner_name"`
	GroupName string `db:"group_name"`
	FilePerm  string `db:"file_perm"`
	ModTime   int64  `db:"mod_time"`
	FileHash  []byte `db:"file_hash"`
	HashAlgo  string `db:"hash_algo"`
}

// ProofRow 对应 proof_records 表的一行。
type ProofRow struct {
	ID           int64  `db:"id"`
	CertID       int64  `db:"cert_id"`
	ChainIndex   int    `db:"chain_index"`
	PrevHash     []byte `db:"prev_hash"`
	DataHash     []byte `db:"data_hash"`
	Signature    []byte `db:"signature"`
	DeviceUID    []byte `db:"device_uid"`
	SigTimestamp int64  `db:"sig_timestamp"`
	HashAlgo     string `db:"hash_algo"`
}

// ============================================================
// 接口定义
// ============================================================

// CertifiedRepository 定义存证操作记录的访问接口。
type CertifiedRepository interface {
	Insert(ctx context.Context, row *CertifiedOpRow) (int64, error)
	Query(ctx context.Context, page, limit int, logType, username string) ([]CertifiedOpRow, int64, error)
	GetByID(ctx context.Context, id int64) (*CertifiedOpRow, error)
}

// ProofRepository 定义存证哈希链的访问接口。
type ProofRepository interface {
	Insert(ctx context.Context, row *ProofRow) error
	GetLastHash(ctx context.Context) ([]byte, error)
	NextChainIndex(ctx context.Context) (int, error)
	GetByCertID(ctx context.Context, certID int64) (*ProofRow, error)
	QueryRange(ctx context.Context, from, to int) ([]ProofRow, error)
	DeleteByCertID(ctx context.Context, certID int64) error
}

// ============================================================
// sqlx 实现
// ============================================================

type certifiedRepo struct {
	db *sqlx.DB
}

// NewCertifiedRepo 创建存证操作记录的 sqlx 仓库实现。
func NewCertifiedRepo(db *sqlx.DB) CertifiedRepository {
	return &certifiedRepo{db: db}
}

func (r *certifiedRepo) Insert(ctx context.Context, row *CertifiedOpRow) (int64, error) {
	const q = `INSERT INTO certified_operations
		(timestamp, type, user_name, user_uid, action, path, dest_path, detail,
		 file_name, is_dir, file_size, mime_type, owner_uid, owner_name,
		 group_name, file_perm, mod_time, file_hash, hash_algo)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	res, err := r.db.ExecContext(ctx, q,
		row.Timestamp, row.Type, row.UserName, row.UserUID,
		row.Action, row.Path, row.DestPath, row.Detail,
		row.FileName, row.IsDir, row.FileSize, row.MimeType,
		row.OwnerUID, row.OwnerName, row.GroupName, row.FilePerm,
		row.ModTime, row.FileHash, row.HashAlgo,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *certifiedRepo) Query(ctx context.Context, page, limit int, logType, username string) ([]CertifiedOpRow, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	if logType != "" && logType != "all" {
		where += " AND type = ?"
		args = append(args, logType)
	}
	if username != "" {
		where += " AND user_name = ?"
		args = append(args, username)
	}

	var total int64
	countQ := "SELECT COUNT(*) FROM certified_operations " + where
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	dataQ := "SELECT * FROM certified_operations " + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var rows []CertifiedOpRow
	if err := r.db.SelectContext(ctx, &rows, dataQ, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *certifiedRepo) GetByID(ctx context.Context, id int64) (*CertifiedOpRow, error) {
	var row CertifiedOpRow
	if err := r.db.GetContext(ctx, &row, "SELECT * FROM certified_operations WHERE id = ?", id); err != nil {
		return nil, err
	}
	return &row, nil
}

type proofRepo struct {
	db *sqlx.DB
}

// NewProofRepo 创建存证哈希链的 sqlx 仓库实现。
func NewProofRepo(db *sqlx.DB) ProofRepository {
	return &proofRepo{db: db}
}

func (r *proofRepo) Insert(ctx context.Context, row *ProofRow) error {
	const q = `INSERT INTO proof_records
		(cert_id, chain_index, prev_hash, data_hash, signature,
		 device_uid, sig_timestamp, hash_algo)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, q,
		row.CertID, row.ChainIndex, row.PrevHash, row.DataHash,
		row.Signature, row.DeviceUID, row.SigTimestamp, row.HashAlgo,
	)
	return err
}

func (r *proofRepo) GetLastHash(ctx context.Context) ([]byte, error) {
	var hash []byte
	err := r.db.GetContext(ctx, &hash, "SELECT data_hash FROM proof_records ORDER BY id DESC LIMIT 1")
	if err != nil {
		return nil, nil // 首次调用时表为空，返回 nil 不报错
	}
	return hash, nil
}

func (r *proofRepo) NextChainIndex(ctx context.Context) (int, error) {
	var idx int
	err := r.db.GetContext(ctx, &idx, "SELECT COALESCE(MAX(chain_index), 0) + 1 FROM proof_records")
	if err != nil {
		return 1, nil // 表为空时从 1 开始
	}
	return idx, nil
}

func (r *proofRepo) GetByCertID(ctx context.Context, certID int64) (*ProofRow, error) {
	var row ProofRow
	if err := r.db.GetContext(ctx, &row, "SELECT * FROM proof_records WHERE cert_id = ?", certID); err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *proofRepo) QueryRange(ctx context.Context, from, to int) ([]ProofRow, error) {
	var rows []ProofRow
	err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM proof_records WHERE chain_index BETWEEN ? AND ? ORDER BY chain_index ASC",
		from, to,
	)
	return rows, err
}

func (r *proofRepo) DeleteByCertID(ctx context.Context, certID int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM proof_records WHERE cert_id = ?", certID)
	return err
}
