package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"nas/ldap"
	"nas/logbuf"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 路径校验辅助函数
// ============================================================

// validatePath 根据角色选择路径校验策略，返回规范化路径。
//
// user  → 限制在 /data/{username}/ 子树内（调用 system.ValidatePath）
// admin → 限制在 /data/ 子树内（调用 system.ValidateAdminPath）
//
// 两个校验函数都使用 filepath.Clean 规范化路径后再做前缀匹配，防止路径穿越。
func validatePath(username, role, requestedPath string) (string, error) {
	if role == "admin" {
		return system.ValidateAdminPath(requestedPath)
	}
	return system.ValidatePath(username, requestedPath)
}

// resolveUID 根据角色和目标路径解析文件操作后 chown 的目标 UID。
//
// user  角色：始终返回该用户自己的 UID（操作限定在自己目录下）
// admin 角色：从路径 /data/{targetuser}/... 中提取目标用户名后查 LDAP，
//
//	提取失败时 fallback 返回 0（root），
//	此时 os.Chown(0, 1000) 会将文件送给 root:nas-users，这在 admin
//	操作 /data/ 根目录下的文件时是合理的。
func resolveUID(username, role, requestPath string) int {
	if role == "admin" {
		// 从路径中提取目标用户名，如 /data/alice/photos → "alice"
		target := extractUserFromPath(requestPath)
		if target != "" {
			return lookupUID(target)
		}
		return 0 // 无法提取用户名，fallback root
	}
	return lookupUID(username)
}

// extractUserFromPath 从 /data/{username}/... 路径中提取用户名部分。
// /data/alice/photos/ → "alice"，/data/bob → "bob"，/data → ""
func extractUserFromPath(path string) string {
	trimmed := strings.TrimPrefix(filepath.Clean(path), "/data/")
	if trimmed == "" || strings.HasPrefix(trimmed, "..") {
		return ""
	}
	parts := strings.SplitN(trimmed, "/", 2)
	return parts[0]
}

// lookupUID 通过 LDAP 查询指定用户的 UID，失败时返回 0。
// 连接失败时仅记录日志，不阻塞主流程（chown 失败不影响文件写入成功）。
func lookupUID(username string) int {
	conn, err := ldap.Conn()
	if err != nil {
		return 0
	}
	defer conn.Close()
	uid, _ := ldap.GetUID(conn, username)
	return uid
}

// ============================================================
// 文件操作 Handler（用户域 /files/*，所有角色共用）
// ============================================================

// ListFiles 列出目录下的文件和子目录。
//
// admin 角色：可以列出 /data/ 下任意路径（所有用户目录可见）
// user  角色：限制在 /data/{username}/ 子树内
//
// 查询参数 path 为空时默认路径：
//
//	admin → /data/
//	user  → /data/{username}/
//
// @Summary      List directory contents
// @Description  Return file list for the given path. Admin users can browse all /data/.
// @Tags         files
// @Produce      json
// @Security     BearerAuth
// @Param        path query string false "Directory path"
// @Success      200 {object} ListFilesResponse "Directory listing"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Failure      404 {object} ErrorResponse "Path not found"
// @Router       /api/files [get]
func ListFiles(c *gin.Context) {
	username := c.GetString("username")
	role := c.GetString("role")
	path := c.Query("path")

	if path == "" {
		if role == "admin" {
			path = "/data"
		} else {
			path = filepath.Join("/data", username)
		}
	}

	cleanPath, err := validatePath(username, role, path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	files, err := system.ListDir(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "path not found"})
		} else if os.IsPermission(err) {
			c.JSON(http.StatusForbidden, gin.H{"error": "permission denied"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "list failed"})
		}
		return
	}

	if files == nil {
		files = []system.FileInfo{}
	}
	c.JSON(http.StatusOK, ListFilesResponse{Path: cleanPath, Files: files})
}

// DownloadFile 下载文件。admin 可下载任意 /data/ 下的文件。
//
// @Summary      Download a file
// @Description  Stream file content as binary data.
// @Tags         files
// @Produce      octet-stream
// @Security     BearerAuth
// @Param        path query string true "File path to download"
// @Success      200 {file} binary "File content"
// @Failure      400 {object} ErrorResponse "Path required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Failure      404 {object} ErrorResponse "File not found"
// @Router       /api/files/download [get]
func DownloadFile(c *gin.Context) {
	username := c.GetString("username")
	role := c.GetString("role")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	cleanPath, err := validatePath(username, role, path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	reader, size, err := system.OpenFile(cleanPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	defer reader.Close()

	c.Header("Content-Disposition", "attachment; filename="+filepath.Base(cleanPath))
	c.DataFromReader(http.StatusOK, size, "application/octet-stream", reader, nil)
}

// UploadFile 上传文件。
//
// admin 可以上传到 /data/ 下任意目录，创建的文件 chown 给目标用户。
// user  只能上传到自己的 /data/{username}/ 目录。
//
// @Summary      Upload a file
// @Description  Upload a file via multipart form.
// @Tags         files
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file formData file true "File to upload"
// @Param        path formData string false "Target directory"
// @Success      200 {object} OKPathResponse "Upload successful"
// @Failure      400 {object} ErrorResponse "File field required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Router       /api/files/upload [post]
func UploadFile(c *gin.Context) {
	username := c.GetString("username")
	role := c.GetString("role")
	targetDir := c.PostForm("path")
	if targetDir == "" {
		if role == "admin" {
			targetDir = "/data"
		} else {
			targetDir = filepath.Join("/data", username)
		}
	}

	cleanDir, err := validatePath(username, role, targetDir)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file field required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "open upload failed"})
		return
	}
	defer src.Close()

	destPath := filepath.Join(cleanDir, file.Filename)
	// 确定文件 owner：admin 从目标路径提取用户名查 UID，user 用自己的 UID
	ownerUID := resolveUID(username, role, destPath)
	if err := system.WriteFile(destPath, src, ownerUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write failed"})
		return
	}

	logbuf.Default.Append(logbuf.Entry{
		Type:   "file",
		User:   username,
		Action: "upload",
		Path:   destPath,
		Size:   file.Size,
	})
	fileLogRepo().Insert(c.Request.Context(), toLogEntry("file", username, "upload", destPath, file.Size, ""))

	c.JSON(http.StatusOK, OKPathResponse{OK: true, Path: destPath})
}

// Mkdir 创建目录。
//
// @Summary      Create a directory
// @Description  Create a new directory.
// @Tags         files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body MkdirRequest true "Directory path"
// @Success      200 {object} OKPathResponse "Directory created"
// @Failure      400 {object} ErrorResponse "Path required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Router       /api/files/mkdir [post]
func Mkdir(c *gin.Context) {
	username := c.GetString("username")
	role := c.GetString("role")
	var req MkdirRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	cleanPath, err := validatePath(username, role, req.Path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	// 确定目录 owner
	ownerUID := resolveUID(username, role, cleanPath)
	if err := system.MakeDir(cleanPath, ownerUID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mkdir failed"})
		return
	}

	logbuf.Default.Append(logbuf.Entry{
		Type:   "file",
		User:   username,
		Action: "mkdir",
		Path:   cleanPath,
	})
	fileLogRepo().Insert(c.Request.Context(), toLogEntry("file", username, "mkdir", cleanPath, 0, ""))

	c.JSON(http.StatusOK, OKPathResponse{OK: true, Path: cleanPath})
}

// DeleteFile 删除文件或目录。
//
// @Summary      Delete a file or directory
// @Description  Permanently remove the specified file or directory.
// @Tags         files
// @Produce      json
// @Security     BearerAuth
// @Param        path query string true "Path to delete"
// @Success      200 {object} OKResponse "Deleted"
// @Failure      400 {object} ErrorResponse "Path required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Router       /api/files [delete]
func DeleteFile(c *gin.Context) {
	username := c.GetString("username")
	role := c.GetString("role")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	cleanPath, err := validatePath(username, role, path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.DeletePath(cleanPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}

	logbuf.Default.Append(logbuf.Entry{
		Type:   "file",
		User:   username,
		Action: "delete",
		Path:   cleanPath,
	})
	fileLogRepo().Insert(c.Request.Context(), toLogEntry("file", username, "delete", cleanPath, 0, ""))

	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// MoveFile 移动或重命名文件/目录。from 和 to 都经过路径校验。
//
// @Summary      Move or rename a file/directory
// @Description  Move a file or directory from one path to another.
// @Tags         files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body MoveFileRequest true "Source and destination paths"
// @Success      200 {object} OKResponse "Moved"
// @Failure      400 {object} ErrorResponse "From and to required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Router       /api/files/move [post]
func MoveFile(c *gin.Context) {
	username := c.GetString("username")
	role := c.GetString("role")
	var req MoveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to required"})
		return
	}

	// from 和 to 都必须独立通过路径校验
	cleanFrom, err := validatePath(username, role, req.From)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	cleanTo, err := validatePath(username, role, req.To)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.MovePath(cleanFrom, cleanTo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "move failed"})
		return
	}

	logbuf.Default.Append(logbuf.Entry{
		Type:   "file",
		User:   username,
		Action: "move",
		Path:   cleanFrom,
		Detail: "→ " + cleanTo,
	})
	fileLogRepo().Insert(c.Request.Context(), toLogEntry("file", username, "move", cleanFrom, 0, "→ "+cleanTo))

	c.JSON(http.StatusOK, OKResponse{OK: true})
}
