package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"nas/handler"
	"nas/ldap"
	jwtpkg "nas/pkg/jwt"

	"github.com/gin-gonic/gin"
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

	r.POST("/register", handler.Register)
	r.POST("/login", handler.Login)
	r.GET("/validate-token", jwtMiddleware(), handler.ValidateToken)
	r.POST("/internal/verify-password", handler.VerifyPassword)

	authed := r.Group("/", jwtMiddleware())
	authed.POST("/share/permission", handler.SetPermission)

	log.Println("authd listening on :8080")
	log.Fatal(r.Run(":8080"))
}
