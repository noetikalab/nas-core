package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nas/repository"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// ListLogs 返回分页的存证操作日志，数据来源为 certified_operations 表。
//
// 查询参数：
//   - page：页码，默认 1
//   - limit：每页条数，默认 20，最大 100
//   - type：日志类型过滤（"file" | "auth" | "system"），空字符串表示不过滤
//   - username：操作者过滤，空字符串表示不过滤
func ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	logType := c.Query("type")
	username := c.Query("username")

	rows, total, err := certifiedRepo().Query(c.Request.Context(), page, limit, logType, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "log query failed"})
		return
	}

	entries := make([]CertifiedOperation, len(rows))
	for i, r := range rows {
		entries[i] = toCertifiedOperation(&r)
	}

	c.JSON(http.StatusOK, LogListResponse{
		Entries:    entries,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: (int(total) + limit - 1) / limit,
	})
}

// buildCertifiedOp 构建完整的存证操作记录。
// 收集文件元信息（os.Stat + 所有者查询），计算文件哈希（SHA-256，puf-agent 就绪后改为 SM3）。
// 返回 repository.CertifiedOpRow 供 Insert 使用。
func buildCertifiedOp(username, action, path string, size int64, destPath, detail string) *repository.CertifiedOpRow {
	row := &repository.CertifiedOpRow{
		Timestamp: time.Now().UnixNano(),
		Type:      "file",
		UserName:  username,
		Action:    action,
		Path:      path,
		DestPath:  destPath,
		Detail:    detail,
		FileName:  filepath.Base(path),
	}

	// 收集文件元信息（非 delete 操作需要 stat 文件）
	if action != "delete" {
		info, err := os.Stat(path)
		if err == nil {
			row.IsDir = info.IsDir()
			row.FileSize = info.Size()
			row.ModTime = info.ModTime().UnixNano()
			row.FilePerm = system.PermStr(info.Mode())
			row.MimeType = inferMime(filepath.Ext(path))
			if stat, ok := info.Sys().(*syscall.Stat_t); ok {
				row.OwnerUID = int(stat.Uid)
			}
		}
	}
	row.FileName = filepath.Base(path)
	row.GroupName = "nas-users"

	// 推断所有者用户名（从路径提取，如 /data/alice/xxx → "alice"）
	if row.OwnerUID != 0 {
		row.OwnerName = extractUserFromPath(path)
		if row.OwnerName == "shared" {
			row.OwnerName = "root"
		}
	}

	// 计算文件内容哈希（仅 upload/download 操作，目录/delete/move 不需要）
	if (action == "upload" || action == "download") && !row.IsDir {
		row.FileHash = sha256File(path)
		row.HashAlgo = "SHA-256"
	}

	// non-file actions
	if action == "mkdir" {
		row.IsDir = true
	}

	return row
}

// recordCertifiedOp 写入存证操作记录 + 补哈希链。
// 这是 handler 层统一的存证写入入口，替代旧 logbuf + logRepo。
// 返回 certID，供调用方后续签名时使用。
func recordCertifiedOp(c *gin.Context, username, action, path string, size int64, destPath, detail string) int64 {
	row := buildCertifiedOp(username, action, path, size, destPath, detail)

	certID, err := certifiedRepo().Insert(c.Request.Context(), row)
	if err != nil {
		return 0
	}

	// 补哈希链
	prevHash, _ := proofRepo().GetLastHash(c.Request.Context())
	chainIdx, _ := proofRepo().NextChainIndex(c.Request.Context())
	dataHash := chainHash(row, prevHash)

	proofRepo().Insert(c.Request.Context(), &repository.ProofRow{
		CertID:     certID,
		ChainIndex: chainIdx,
		PrevHash:   prevHash,
		DataHash:   dataHash,
		HashAlgo:   "SHA-256",
	})

	return certID
}

// -------------------- 辅助函数 --------------------

// sha256File 计算文件的 SHA-256 哈希。失败时返回 nil。
func sha256File(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	h := sha256.Sum256(data)
	return h[:]
}

// inferMime 根据文件扩展名推断 MIME 类型。
// 仅覆盖常见类型，其他返回 "application/octet-stream"。
func inferMime(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	case ".txt", ".md", ".log":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".zip":
		return "application/zip"
	case ".tar", ".gz", ".tgz":
		return "application/gzip"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".ppt", ".pptx":
		return "application/vnd.ms-powerpoint"
	default:
		return "application/octet-stream"
	}
}

// chainHash 计算哈希链上当前节点的哈希值。
// 串联操作记录所有关键字段 + 前一条记录的哈希，再做 SHA-256。
func chainHash(row *repository.CertifiedOpRow, prevHash []byte) []byte {
	data := fmt.Sprintf("%d:%s:%s:%s:%s:%s:%x",
		row.Timestamp, row.UserName, row.Action, row.Path, row.FileName, row.MimeType, row.FileHash)
	h := sha256.New()
	h.Write(prevHash)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// toCertifiedOperation 将数据库行转换为 API 响应 DTO。
func toCertifiedOperation(r *repository.CertifiedOpRow) CertifiedOperation {
	return CertifiedOperation{
		ID:        r.ID,
		Timestamp: r.Timestamp,
		Type:      r.Type,
		UserName:  r.UserName,
		UserUID:   r.UserUID,
		Action:    r.Action,
		Path:      r.Path,
		DestPath:  r.DestPath,
		Detail:    r.Detail,
		FileName:  r.FileName,
		IsDir:     r.IsDir,
		FileSize:  r.FileSize,
		MimeType:  r.MimeType,
		OwnerUID:  r.OwnerUID,
		OwnerName: r.OwnerName,
		GroupName: r.GroupName,
		FilePerm:  r.FilePerm,
		ModTime:   r.ModTime,
		FileHash:  base64.StdEncoding.EncodeToString(r.FileHash),
		HashAlgo:  r.HashAlgo,
	}
}

// -------------------- Repository 依赖注入 --------------------

var certifiedRepoInstance repository.CertifiedRepository
var proofRepoInstance repository.ProofRepository

// SetCertifiedRepos 设置全局存证仓库实例。在程序启动时由 main.go 调用一次。
func SetCertifiedRepos(cr repository.CertifiedRepository, pr repository.ProofRepository) {
	certifiedRepoInstance = cr
	proofRepoInstance = pr
}

func certifiedRepo() repository.CertifiedRepository {
	if certifiedRepoInstance == nil {
		panic("CertifiedRepository not set")
	}
	return certifiedRepoInstance
}

func proofRepo() repository.ProofRepository {
	if proofRepoInstance == nil {
		panic("ProofRepository not set")
	}
	return proofRepoInstance
}

// --- 向后兼容：保留 logRepo/fileLogRepo 别名（Dashboard 等沿用） ---
// 这些已不再需要，但 Dashboard handler 目前引用它们。
// 待 Dashboard handler 重构后移除。

var logRepoInstance repository.LogRepository

// SetLogRepo 设置全局日志仓库实例（向后兼容，Dashboard handler 仍依赖此注入）。
func SetLogRepo(r repository.LogRepository) {
	logRepoInstance = r
}

func logRepo() repository.LogRepository { return logRepoInstance }
func fileLogRepo() repository.LogRepository { return logRepo() }
