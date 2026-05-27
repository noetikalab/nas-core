package handler

import (
	"net/http"

	"nas/system"

	"github.com/gin-gonic/gin"
)

// SetPermission
// @Summary      Set file share permission
// @Description  Grant or revoke POSIX ACL access for another user.
// @Description  Actions: "readonly" (r-x), "readwrite" (rwx), "remove" (revoke all).
// @Tags         share
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body SetPermissionRequest true "Permission details"
// @Success      200 {object} OKResponse "Permission set"
// @Failure      400 {object} ErrorResponse "Invalid input"
// @Failure      401 {object} ErrorResponse "Unauthorized"
// @Router       /api/share/permission [post]
func SetPermission(c *gin.Context) {
	var req SetPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	if req.Action == "remove" {
		system.RemoveACL(req.Path, req.TargetUser)
		c.JSON(http.StatusOK, OKResponse{OK: true})
		return
	}

	perm := "rwx"
	if req.Action == "readonly" || req.Readonly {
		perm = "r-x"
	}
	system.SetACL(req.Path, req.TargetUser, perm)
	c.JSON(http.StatusOK, OKResponse{OK: true})
}
