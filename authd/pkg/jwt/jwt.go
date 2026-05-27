// Package jwt 提供 NAS 系统的 JWT 令牌签发和解析功能。
//
// 令牌中包含两个自定义 claim：
//   - sub：用户名（标准 subject claim）
//   - role：用户角色（"admin" 或 "user"），来自 LDAP employeeType 属性
//
// 令牌有效期 24 小时，密钥由环境变量 JWT_SECRET 注入。
package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Secret 是 JWT 签名密钥，由 main.go 从环境变量 JWT_SECRET 读取后注入。
// 为空时 JWT 库会使用空密钥签名（不安全），生产环境务必设置。
var Secret []byte

// Sign 签发包含用户名和角色的 JWT 令牌，有效期 24 小时。
//
// 参数：
//   - username：来自 LDAP uid 的用户名
//   - role：用户角色（"admin" 或 "user"），由 Register 根据首次注册决定，
//     或 Login 时从 LDAP employeeType 读取
//
// 返回签名的 JWT 字符串，或签名错误。
func Sign(username, role string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  username,                              // 标准 subject claim
		"role": role,                                  // 自定义角色 claim
		"exp":  time.Now().Add(24 * time.Hour).Unix(), // 24 小时后过期
		"iat":  time.Now().Unix(),                     // 签发时间
	}).SignedString(Secret)
}

// Parse 解析并验证 JWT 令牌，返回用户名、角色和验证结果。
//
// token 必须是未经 Bearer 前缀剥离的原始 JWT 字符串，
// 通常由 Authorization header 解析得来。
//
// 返回值：
//   - username：令牌中的用户名，验证失败时为空
//   - role：令牌中的角色，验证失败时为空
//   - ok：true 表示令牌有效
func Parse(tokenStr string) (username, role string, ok bool) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return Secret, nil
	})
	if err != nil || !token.Valid {
		return "", "", false
	}
	claims := token.Claims.(jwt.MapClaims)
	sub, ok1 := claims["sub"].(string)
	// role 可能不存在（旧版本令牌），不存在时返回空字符串
	r, _ := claims["role"].(string)
	return sub, r, ok1
}
