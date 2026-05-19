package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"nas/system"

	"github.com/gin-gonic/gin"
)

// ListFiles
// @Summary      List directory contents
// @Description  Return file list for the given path. Defaults to user's home directory.
// @Tags         files
// @Produce      json
// @Security     BearerAuth
// @Param        path query string false "Directory path (default: /data/<username>)"
// @Success      200 {object} ListFilesResponse "Directory listing"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Failure      404 {object} ErrorResponse "Path not found"
// @Router       /files [get]
func ListFiles(c *gin.Context) {
	username := c.GetString("username")
	path := c.Query("path")
	if path == "" {
		path = filepath.Join("/data", username)
	}

	cleanPath, err := system.ValidatePath(username, path)
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

// DownloadFile
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
// @Router       /files/download [get]
func DownloadFile(c *gin.Context) {
	username := c.GetString("username")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	cleanPath, err := system.ValidatePath(username, path)
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

// UploadFile
// @Summary      Upload a file
// @Description  Upload a file via multipart form. Target directory defaults to user's home.
// @Tags         files
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file formData file true "File to upload"
// @Param        path formData string false "Target directory (default: /data/<username>)"
// @Success      200 {object} OKPathResponse "Upload successful"
// @Failure      400 {object} ErrorResponse "File field required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Router       /files/upload [post]
func UploadFile(c *gin.Context) {
	username := c.GetString("username")
	targetDir := c.PostForm("path")
	if targetDir == "" {
		targetDir = filepath.Join("/data", username)
	}

	cleanDir, err := system.ValidatePath(username, targetDir)
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
	if err := system.WriteFile(destPath, src); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "write failed"})
		return
	}

	c.JSON(http.StatusOK, OKPathResponse{OK: true, Path: destPath})
}

// Mkdir
// @Summary      Create a directory
// @Description  Create a new directory under the user's data path.
// @Tags         files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body MkdirRequest true "Directory path"
// @Success      200 {object} OKPathResponse "Directory created"
// @Failure      400 {object} ErrorResponse "Path required"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Failure      403 {object} ErrorResponse "Access denied"
// @Router       /files/mkdir [post]
func Mkdir(c *gin.Context) {
	username := c.GetString("username")
	var req MkdirRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	cleanPath, err := system.ValidatePath(username, req.Path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.MakeDir(cleanPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mkdir failed"})
		return
	}
	c.JSON(http.StatusOK, OKPathResponse{OK: true, Path: cleanPath})
}

// DeleteFile
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
// @Router       /files [delete]
func DeleteFile(c *gin.Context) {
	username := c.GetString("username")
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	cleanPath, err := system.ValidatePath(username, path)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.DeletePath(cleanPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed"})
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}

// MoveFile
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
// @Router       /files/move [post]
func MoveFile(c *gin.Context) {
	username := c.GetString("username")
	var req MoveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to required"})
		return
	}

	cleanFrom, err := system.ValidatePath(username, req.From)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	cleanTo, err := system.ValidatePath(username, req.To)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	if err := system.MovePath(cleanFrom, cleanTo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "move failed"})
		return
	}
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
