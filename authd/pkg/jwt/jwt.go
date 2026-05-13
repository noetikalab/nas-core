package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var Secret []byte

func Sign(username string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": username,
		"exp": time.Now().Add(24 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}).SignedString(Secret)
}

func Parse(tokenStr string) (username string, ok bool) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		return Secret, nil
	})
	if err != nil || !token.Valid {
		return "", false
	}
	claims := token.Claims.(jwt.MapClaims)
	sub, ok := claims["sub"].(string)
	return sub, ok
}
