// Package repository 定义数据访问层（Repository 模式），
// 将数据库操作抽象为接口，Handler 通过接口依赖注入使用，
// 便于测试时替换为 mock 实现。
package repository

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// LogEntry 操作日志数据库行，对应 operation_logs 表的一行记录。
// db 标签用于 sqlx 的 StructScan / NamedExec 字段映射。
type LogEntry struct {
	ID        int64     `db:"id"`
	Timestamp time.Time `db:"timestamp"`
	Type      string    `db:"type"`     // "file" | "auth" | "system"
	Username  string    `db:"username"` // 操作者用户名
	Action    string    `db:"action"`   // "upload" | "delete" | "mkdir" | "move" | "download"
	Path      string    `db:"path"`     // 操作涉及的文件/目录路径
	Size      int64     `db:"size"`     // 文件操作时的大小（字节），非文件操作为 0
	Detail    string    `db:"detail"`   // 补充说明，如移动操作的目标路径
}

// LogRepository 定义操作日志的数据访问接口。
// 实现必须保证并发安全（本仓库的 sqlx 实现通过 SQLite 串行化保证）。
type LogRepository interface {
	Insert(ctx context.Context, entry *LogEntry) error
	Query(ctx context.Context, page, limit int, logType, username string) ([]LogEntry, int64, error)
}

// logRepo 是 LogRepository 的 sqlx 实现。
// db 是 sqlx 扩展的标准库 sql.DB，内部使用连接池。
type logRepo struct {
	db *sqlx.DB
}

// NewLogRepo 创建一个基于 sqlx 的日志仓库实现。
// db 参数由 db.Connect() 返回，在 main.go 中初始化后注入。
func NewLogRepo(db *sqlx.DB) LogRepository {
	return &logRepo{db: db}
}

// Insert 将一条操作日志写入 operation_logs 表。
// entry.Timestamp 由调用方设置（通常为 time.Now()），
// 这样与 logbuf 的环形缓冲保持时间一致。
func (r *logRepo) Insert(ctx context.Context, entry *LogEntry) error {
	const q = `INSERT INTO operation_logs (timestamp, type, username, action, path, size, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, q,
		entry.Timestamp, entry.Type, entry.Username,
		entry.Action, entry.Path, entry.Size, entry.Detail,
	)
	return err
}

// Query 分页查询操作日志，支持按日志类型和用户名过滤。
//
// 参数：
//   - page, limit：分页参数，page 从 1 开始，limit 为每页条数
//   - logType：日志类型过滤，空字符串或 "all" 表示不过滤
//   - username：用户名过滤，空字符串表示不过滤
//
// 返回值：
//   - []LogEntry：当前页的日志条目，按时间倒序（最新在前）
//   - int64：符合条件的总记录数（用于计算 totalPages）
func (r *logRepo) Query(ctx context.Context, page, limit int, logType, username string) ([]LogEntry, int64, error) {
	// 动态拼接 WHERE 条件
	where := "WHERE 1=1"
	args := []any{}
	if logType != "" && logType != "all" {
		where += " AND type = ?"
		args = append(args, logType)
	}
	if username != "" {
		where += " AND username = ?"
		args = append(args, username)
	}

	// 先查总数，用于前端分页控件
	var total int64
	countQ := "SELECT COUNT(*) FROM operation_logs " + where
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, err
	}

	// 再查当前页数据，按时间倒序
	offset := (page - 1) * limit
	dataQ := "SELECT * FROM operation_logs " + where + " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	var rows []LogEntry
	if err := r.db.SelectContext(ctx, &rows, dataQ, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}
