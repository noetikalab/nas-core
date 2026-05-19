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

	_ "nas/docs"
	"nas/handler"
	"nas/ldap"
	jwtpkg "nas/pkg/jwt"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		username, ok := jwtpkg.Parse(tokenStr)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Set("username", username)
		c.Next()
	}
}

func main() {
	jwtpkg.Secret = []byte(os.Getenv("JWT_SECRET"))
	ldap.URL = os.Getenv("LDAP_URL")
	ldap.AdminDN = os.Getenv("LDAP_ADMIN_DN")
	ldap.AdminPW = os.Getenv("LDAP_ADMIN_PW")
	ldap.UsersDN = os.Getenv("LDAP_USERS_DN")
	ldap.DomainSID = os.Getenv("DOMAIN_SID")

	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.POST("/register", handler.Register)
	r.POST("/login", handler.Login)
	r.GET("/validate-token", jwtMiddleware(), handler.ValidateToken)
	r.POST("/internal/verify-password", handler.VerifyPassword)

	authed := r.Group("/", jwtMiddleware())
	authed.POST("/share/permission", handler.SetPermission)
	authed.GET("/files", handler.ListFiles)
	authed.GET("/files/download", handler.DownloadFile)
	authed.POST("/files/upload", handler.UploadFile)
	authed.POST("/files/mkdir", handler.Mkdir)
	authed.DELETE("/files", handler.DeleteFile)
	authed.POST("/files/move", handler.MoveFile)

	log.Println("authd listening on :8080")
	log.Fatal(r.Run(":8080"))
}
