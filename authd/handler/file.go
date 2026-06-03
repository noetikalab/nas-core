package handler

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"nas/ldap"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// ============================================================
// 路径校验辅助函数
// ============================================================

// validatePath 根据角色选择路径校验策略，返回规范化路径。
//
// user  → 限制在 /data/{username}/ 子树内，或 /data/shared/ 共享目录
// admin → 限制在 /data/ 子树内（调用 system.ValidateAdminPath）
//
// 两个校验函数都使用 filepath.Clean 规范化路径后再做前缀匹配，防止路径穿越。
func validatePath(username, role, requestedPath string) (string, error) {
	if role == "admin" {
		return system.ValidateAdminPath(requestedPath)
	}
	clean := filepath.Clean(requestedPath)
	// 普通用户可读共享目录 /data/shared/
	if strings.HasPrefix(clean, "/data/shared") {
		return clean, nil
	}
	return system.ValidatePath(username, requestedPath)
}

// validateWritePath 在 validatePath 基础上额外限制普通用户不能写共享目录。
// 共享目录默认只读，除非 admin 通过 POSIX ACL 授予了该用户 rwx 权限。
// 读操作（List/Download）使用 validatePath，写操作（Upload/Mkdir/Delete/Move）使用此函数。
func validateWritePath(username, role, requestedPath string) (string, error) {
	clean, err := validatePath(username, role, requestedPath)
	if err != nil {
		return "", err
	}
	// 非 admin 对共享目录的写操作：检查 POSIX ACL 是否单独授权
	if role != "admin" && strings.HasPrefix(clean, "/data/shared") && clean != "/data/shared" {
		if !hasACLWrite(clean, username) {
			return "", fmt.Errorf("shared directory is read-only")
		}
	}
	return clean, nil
}

// hasACLWrite 检查指定用户在路径上是否有 POSIX ACL 写权限（rwx）。
// 通过 getfacl 解析 user:{uid}:rwx 条目判断。
// 使用数字 UID 匹配而非用户名——容器内 setfacl 无法解析 LDAP 用户名，
// ACL 中存储的是数字 UID（由 system.ResolveACLUser 转换）。
// 如果路径本身不存在（mkdir、move 目标等新建操作），向上查找
// 最近已存在父目录的 ACL 作为判断依据。
func hasACLWrite(path, username string) bool {
	checkPath := path
	if _, err := os.Stat(checkPath); os.IsNotExist(err) {
		checkPath = filepath.Dir(checkPath)
	}
	out, err := exec.Command("getfacl", "-c", checkPath).CombinedOutput()
	if err != nil {
		return false
	}
	// 将用户名转换为 UID，匹配 ACL 中的数字 UID 条目（如 user:1002:rwx）
	uid := lookupUID(username)
	if uid == 0 {
		return false
	}
	target := fmt.Sprintf("user:%d:rwx", uid)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}

// resolveUID 根据角色和目标路径解析文件操作后 chown 的目标 UID。
//
// user  角色：始终返回该用户自己的 UID（操作限定在自己目录下）
// admin 角色：从路径 /data/{targetuser}/... 中提取目标用户名后查 LDAP，
//
//	提取失败时 fallback 返回 0（root），
//	此时 os.Chown(0, 1000) 会将文件送给 root:nas-users，这在 admin
//	操作 /data/ 根目录下的文件时是合理的。
//	共享目录 /data/shared/ 下的文件也采用 root:nas-users。
func resolveUID(username, role, requestPath string) int {
	if role == "admin" || strings.HasPrefix(requestPath, "/data/shared") {
		// 从路径中提取目标用户名，如 /data/alice/photos → "alice"
		target := extractUserFromPath(requestPath)
		if target != "" && target != "shared" {
			return lookupUID(target)
		}
		return 0 // shared 目录或无法提取：fallback root
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

	cleanDir, err := validateWritePath(username, role, targetDir)
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

		recordCertifiedOp(c, username, "upload", destPath, file.Size, "", "")

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

	cleanPath, err := validateWritePath(username, role, req.Path)
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

	recordCertifiedOp(c, username, "mkdir", cleanPath, 0, "", "")

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

	cleanPath, err := validateWritePath(username, role, path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.DeletePath(cleanPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}

	recordCertifiedOp(c, username, "delete", cleanPath, 0, "", "")

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
	cleanFrom, err := validateWritePath(username, role, req.From)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	cleanTo, err := validateWritePath(username, role, req.To)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.MovePath(cleanFrom, cleanTo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "move failed"})
		return
	}

	recordCertifiedOp(c, username, "move", cleanFrom, 0, cleanTo, "→ "+cleanTo)

	c.JSON(http.StatusOK, OKResponse{OK: true})
}
