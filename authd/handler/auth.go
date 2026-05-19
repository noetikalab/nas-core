package handler

import (
	"net/http"

	"nas/ldap"
	jwtpkg "nas/pkg/jwt"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// Register
// @Summary      Register a new user
// @Description  Create a user in LDAP and Linux system, then return a JWT token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body RegisterRequest true "Registration details"
// @Success      200 {object} RegisterResponse "Registration successful"
// @Failure      400 {object} ErrorResponse "Invalid input"
// @Failure      409 {object} ErrorResponse "Username already taken"
// @Failure      503 {object} ErrorResponse "LDAP unavailable"
// @Router       /register [post]
func Register(c *gin.Context) {
	var req RegisterRequest
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
	c.JSON(http.StatusOK, RegisterResponse{Token: token, UID: uid})
}

// Login
// @Summary      Login
// @Description  Authenticate with LDAP bind and return a JWT token valid for 24 hours.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body LoginRequest true "Login credentials"
// @Success      200 {object} LoginResponse "Login successful"
// @Failure      400 {object} ErrorResponse "Invalid input"
// @Failure      401 {object} ErrorResponse "Wrong credentials"
// @Router       /login [post]
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}
	if err := ldap.Bind(req.Username, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong credentials"})
		return
	}
	token, _ := jwtpkg.Sign(req.Username)
	c.JSON(http.StatusOK, LoginResponse{Token: token})
}

// ValidateToken
// @Summary      Validate JWT token
// @Description  Check whether the provided JWT token is valid and return the username.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} ValidateTokenResponse "Token is valid"
// @Failure      401 {object} ErrorResponse "Invalid or expired token"
// @Router       /validate-token [get]
func ValidateToken(c *gin.Context) {
	c.JSON(http.StatusOK, ValidateTokenResponse{Valid: true, Username: c.GetString("username")})
}

// VerifyPassword
// @Summary      Verify password (internal)
// @Description  Internal endpoint for Samba/WebDAV password verification. Returns success=false for wrong password.
// @Tags         internal
// @Accept       json
// @Produce      json
// @Param        request body VerifyPasswordRequest false "Credentials"
// @Success      200 {object} VerifyPasswordResponse "Password correct"
// @Failure      503 {object} ErrorResponse "LDAP unavailable"
// @Router       /internal/verify-password [post]
func VerifyPassword(c *gin.Context) {
	var req VerifyPasswordRequest
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
	c.JSON(http.StatusOK, VerifyPasswordResponse{Success: true, UID: uid, GID: gid})
}
