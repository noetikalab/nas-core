package handler

import (
	"net/http"

	"nas/system"

	"github.com/gin-gonic/gin"
)

func SetPermission(c *gin.Context) {
	var req struct {
		Path       string `json:"path" binding:"required"`
		TargetUser string `json:"target_user" binding:"required"`
		Action     string `json:"action"` // "readonly", "readwrite", "remove"
		Readonly   bool   `json:"readonly"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	if req.Action == "remove" {
		system.RemoveACL(req.Path, req.TargetUser)
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	perm := "rwx"
	if req.Action == "readonly" || req.Readonly {
		perm = "r-x"
	}
	system.SetACL(req.Path, req.TargetUser, perm)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
