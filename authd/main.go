// @title           NAS Authd API
// @version         1.0
// @description     NAS multi-protocol unified authentication and file management service.
// @description     Register or login to obtain a JWT token, then use it to access protected endpoints.
// @contact.name    NAS Team
// @host            localhost:8080
// @BasePath        /
//
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer " followed by your JWT token. Example: Bearer eyJhbGciOiJIUzI1NiIs...

package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"nas/db"
	_ "nas/docs" // swag init 生成的 OpenAPI 文档（编译时嵌入二进制）
	"nas/handler"
	"nas/ldap"
	"nas/mdns"
	jwtpkg "nas/pkg/jwt"
	"nas/repository"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// jwtMiddleware 从 Authorization header 中提取 Bearer token，解析 JWT，
// 将 username 和 role 注入到 gin.Context 中，供后续 handler 使用。
//
// 解析失败时返回 401 Unauthorized，不会调用后续 handler。
func jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 剥离 "Bearer " 前缀
		tokenStr := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		username, role, ok := jwtpkg.Parse(tokenStr)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		// 注入 context，handler 通过 c.GetString("username") / c.GetString("role") 获取
		c.Set("username", username)
		c.Set("role", role)
		c.Next()
	}
}

// adminOnly 是一个 gin 中间件，检查当前请求的用户角色是否为 "admin"。
// 必须放在 jwtMiddleware 之后使用，因为依赖 c.GetString("role")。
//
// 非管理员请求返回 403 Forbidden。
func adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("role") != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}
		c.Next()
	}
}

func main() {
	// ==================== 1. 环境变量注入到各包全局配置 ====================

	jwtpkg.Secret = []byte(os.Getenv("JWT_SECRET"))
	ldap.URL = os.Getenv("LDAP_URL")
	ldap.AdminDN = os.Getenv("LDAP_ADMIN_DN")
	ldap.AdminPW = os.Getenv("LDAP_ADMIN_PW")
	ldap.UsersDN = os.Getenv("LDAP_USERS_DN")
	ldap.DomainSID = os.Getenv("DOMAIN_SID")

	// ==================== 2. SQLite 数据库初始化 ====================

	// SQLITE_PATH 默认 /data/.nas.db，与用户数据目录同盘，方便备份
	dbPath := os.Getenv("SQLITE_PATH")
	if dbPath == "" {
		dbPath = "/data/.nas.db"
	}

	// 执行嵌入式 SQL 迁移（创建表等），首次运行自动初始化
	if err := db.RunMigrations(dbPath); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	// 建立数据库连接，通过 Repository 模式注入到 handler 层
	sqlDB, err := db.Connect(dbPath)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	handler.SetLogRepo(repository.NewLogRepo(sqlDB))

	// ==================== 3. Router 初始化 ====================

	r := gin.Default()

	// --- Swagger UI（非 API，直接暴露） ---
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// --- 内部接口（Nginx/Samba 回调，不对外暴露） ---
	r.POST("/internal/verify-password", handler.VerifyPassword)

	// --- 公开 API（无需认证） ---
	pub := r.Group("/api")
	pub.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	pub.GET("/device-info", handler.DeviceInfo)
	pub.POST("/register", handler.Register)
	pub.POST("/login", handler.Login)

	// --- 认证 API（需要 JWT，路径范围根据角色自适应） ---
	api := r.Group("/api", jwtMiddleware())
	api.GET("/validate-token", handler.ValidateToken)               // 验证 JWT 有效性
	api.POST("/share/permission", handler.SetPermission)            // 文件分享/ACL 权限设置

	// --- 文件操作 API（需要 JWT） ---
	api.GET("/files", handler.ListFiles)                            // 列出目录
	api.GET("/files/download", handler.DownloadFile)                // 下载文件
	api.POST("/files/upload", handler.UploadFile)                   // 上传文件
	api.POST("/files/mkdir", handler.Mkdir)                         // 新建目录
	api.DELETE("/files", handler.DeleteFile)                        // 删除文件
	api.POST("/files/move", handler.MoveFile)                       // 移动/重命名

	// --- 管理 API（需要 JWT + admin 角色） ---
	admin := r.Group("/api", jwtMiddleware(), adminOnly())
	admin.GET("/dashboard/stats", handler.DashboardStats)           // 系统资源概览
	admin.GET("/dashboard/recent", handler.RecentFiles)             // 最近文件操作
	admin.GET("/users", handler.ListUsers)                          // 用户列表
	admin.GET("/users/count", handler.CountUsers)                   // 用户数
	admin.DELETE("/users/:username", handler.DeleteUser)            // 删除用户
	admin.GET("/logs", handler.ListLogs)                            // 审计日志（分页）
	admin.GET("/services", handler.ListServices)                    // 服务状态

	// ==================== 4. mDNS 局域网发现 ====================
	go func() {
		if err := mdns.Start(8080); err != nil {
			log.Printf("mDNS: %v", err)
		}
	}()

	// ==================== 5. 启动服务 ====================
	log.Println("authd listening on :8080")
	log.Fatal(r.Run(":8080"))
}
