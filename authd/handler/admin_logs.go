package handler

import (
	"net/http"
	"strconv"
	"time"

	"nas/repository"

	"github.com/gin-gonic/gin"
)

// ListLogs 返回分页的操作日志，数据来源为 SQLite（持久化存储）。
//
// 查询参数：
//   - page：页码，默认 1
//   - limit：每页条数，默认 20，最大 100
//   - type：日志类型过滤（"file" | "auth" | "system"），空字符串表示不过滤
//   - username：操作者过滤，空字符串表示不过滤
//
// 返回 paginated JSON，包含当前页条目、总数、总页数等元信息。
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

	rows, total, err := logRepo().Query(c.Request.Context(), page, limit, logType, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "log query failed"})
		return
	}

	entries := make([]LogEntry, len(rows))
	for i, r := range rows {
		entries[i] = LogEntry{
			Timestamp: r.Timestamp.UTC().Format(time.RFC3339),
			Type:      r.Type,
			User:      r.Username,
			Action:    r.Action,
			Detail:    r.Detail,
		}
	}

	c.JSON(http.StatusOK, LogListResponse{
		Entries:    entries,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: (int(total) + limit - 1) / limit, // 向上取整
	})
}

// --- Repository 依赖注入 ---

// logRepoInstance 是全局的日志仓库实例，由 main.go 在初始化时通过 SetLogRepo 注入。
// 使用包级变量实现简单的依赖注入（替代复杂的 DI 框架），
// handler 层不关心具体实现，只依赖 LogRepository 接口。
var logRepoInstance repository.LogRepository

// SetLogRepo 设置全局日志仓库实例。在程序启动时由 main.go 调用一次。
func SetLogRepo(r repository.LogRepository) {
	logRepoInstance = r
}

// logRepo 获取日志仓库实例。如果未设置则 panic（检测启动顺序错误）。
// 文件操作 handler（file.go、admin_files.go）通过此函数获取仓库实例。
func logRepo() repository.LogRepository {
	if logRepoInstance == nil {
		panic("LogRepository not set — call handler.SetLogRepo() during init")
	}
	return logRepoInstance
}

// fileLogRepo 返回日志仓库实例，供文件操作 handler 使用。
// 与 logRepo 指向同一实例，单独函数是为了语义清晰：文件操作日志使用。
func fileLogRepo() repository.LogRepository {
	return logRepo()
}

// toLogEntry 将 handler 层的参数转换为 repository.LogEntry 数据库模型。
//
// Timestamp 在这里设置为当前时间，确保与 logbuf 环形缓冲中的时间戳一致
// （logbuf.Default.Append 也会自动设置 e.Timestamp）。
func toLogEntry(logType, username, action, path string, size int64, detail string) *repository.LogEntry {
	return &repository.LogEntry{
		Timestamp: time.Now(),
		Type:      logType,
		Username:  username,
		Action:    action,
		Path:      path,
		Size:      size,
		Detail:    detail,
	}
}
