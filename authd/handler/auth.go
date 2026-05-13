package handler

import (
	"net/http"

	"nas/ldap"
	jwtpkg "nas/pkg/jwt"
	"nas/system"

	"github.com/gin-gonic/gin"
)

func Register(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ldap unavailable"})
		return
	}
	defer conn.Close()

	uid, err := ldap.NextUID(conn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "uid alloc failed"})
		return
	}
	if err = ldap.AddUser(conn, req.Username, uid, req.Password); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username taken or ldap error"})
		return
	}
	if err = system.CreateUser(req.Username, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "system user creation failed"})
		return
	}
	system.CreateDataDir(req.Username, uid)

	token, _ := jwtpkg.Sign(req.Username)
	c.JSON(http.StatusOK, gin.H{"token": token, "uid": uid})
}

func Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if err := ldap.Bind(req.Username, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong credentials"})
		return
	}
	token, _ := jwtpkg.Sign(req.Username)
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func ValidateToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"valid": true, "username": c.GetString("username")})
}

func VerifyPassword(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	c.ShouldBindJSON(&req)

	if err := ldap.Bind(req.Username, req.Password); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}
	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ldap unavailable"})
		return
	}
	defer conn.Close()

	uid, gid := ldap.GetUID(conn, req.Username)
	c.JSON(http.StatusOK, gin.H{"success": true, "uid": uid, "gid": gid})
}
