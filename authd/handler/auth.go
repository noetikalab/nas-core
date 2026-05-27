package handler

import (
	"net/http"

	"nas/ldap"
	jwtpkg "nas/pkg/jwt"
	"nas/system"

	"github.com/gin-gonic/gin"
)

// Register 注册新用户。
//
// 注册流程：
//   1. 验证请求参数（用户名 + 密码 ≥8 位）
//   2. 从 LDAP 分配下一个可用 UID
//   3. 判断是否为第一个用户 → 自动授予 admin 角色
//   4. 在 LDAP 中创建用户条目（包含 role → employeeType）
//   5. 在 Linux 系统中创建用户和数据目录
//   6. 签发含角色的 JWT 并返回
//
// 首个用户判断：查 LDAP 中 posixAccount 数量，count==0 则表示无用户，
// 自动将 employeeType 设为 "admin"。
//
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
// @Router       /api/register [post]
func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	// 连接 LDAP
	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ldap unavailable"})
		return
	}
	defer conn.Close()

	// 分配下一个可用的 UID（从 1001 开始递增）
	uid, err := ldap.NextUID(conn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "uid alloc failed"})
		return
	}

	// 判断角色：系统中无用户时，第一个注册的用户自动成为管理员
	role := "user"
	count, err := ldap.CountUsers(conn)
	if err == nil && count == 0 {
		role = "admin"
	}

	// 在 LDAP 中创建用户条目
	if err = ldap.AddUser(conn, req.Username, uid, req.Password, role); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username taken or ldap error"})
		return
	}

	// 在 Linux 系统中创建用户（useradd）
	if err = system.CreateUser(req.Username, uid); err != nil {
		// LDAP 已创建但 useradd 失败——生产环境应记录告警并通过定时任务清理不一致
		c.JSON(http.StatusInternalServerError, gin.H{"error": "system user creation failed"})
		return
	}

	// 创建数据目录 /data/{username} 并设置权限
	system.CreateDataDir(req.Username, uid)

	// 签发 JWT，有效期 24 小时
	token, _ := jwtpkg.Sign(req.Username, role)
	c.JSON(http.StatusOK, RegisterResponse{Token: token, UID: uid, Role: role})
}

// Login 用户登录。
//
// 流程：
//   1. 验证用户名和密码（LDAP Bind）
//   2. 从 LDAP employeeType 读取角色
//   3. 签发含角色的 JWT 并返回
//
// @Summary      Login
// @Description  Authenticate with LDAP bind and return a JWT token valid for 24 hours.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request body LoginRequest true "Login credentials"
// @Success      200 {object} LoginResponse "Login successful"
// @Failure      400 {object} ErrorResponse "Invalid input"
// @Failure      401 {object} ErrorResponse "Wrong credentials"
// @Router       /api/login [post]
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
		return
	}

	// LDAP Bind：验证密码
	if err := ldap.Bind(req.Username, req.Password); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "wrong credentials"})
		return
	}

	// 密码正确后，查询用户角色
	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ldap unavailable"})
		return
	}
	defer conn.Close()
	role := ldap.GetUserRole(conn, req.Username)

	// 签发 JWT（含角色 claim）
	token, _ := jwtpkg.Sign(req.Username, role)
	c.JSON(http.StatusOK, LoginResponse{Token: token, Role: role})
}

// ValidateToken
// @Summary      Validate JWT token
// @Description  Check whether the provided JWT token is valid and return the username.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} ValidateTokenResponse "Token is valid"
// @Failure      401 {object} ErrorResponse "Invalid or expired token"
// @Router       /api/validate-token [get]
func ValidateToken(c *gin.Context) {
	// username 由 jwtMiddleware 解析后注入到 gin.Context
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

	// LDAP Bind 验证密码，失败返回 success=false（不返回 401 以免 Samba/WebDAV 报错）
	if err := ldap.Bind(req.Username, req.Password); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false})
		return
	}

	// 密码正确，查询 UID/GID 返回给调用方（Samba/Nginx 需要这些信息）
	conn, err := ldap.Conn()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ldap unavailable"})
		return
	}
	defer conn.Close()

	uid, gid := ldap.GetUID(conn, req.Username)
	c.JSON(http.StatusOK, VerifyPasswordResponse{Success: true, UID: uid, GID: gid})
}
