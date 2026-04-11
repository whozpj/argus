package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 30 * 24 * time.Hour

type argusClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"uid"`
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-in-production"
	}
	return []byte(s)
}

// IssueToken returns a signed HS256 JWT for the given userID, valid for 30 days.
func IssueToken(userID string) (string, error) {
	c := argusClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "argus",
		},
		UserID: userID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(jwtSecret())
}

// ValidateToken parses and validates a JWT string, returning the userID claim.
func ValidateToken(tokenStr string) (string, error) {
	c := &argusClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil || !tok.Valid {
		return "", fmt.Errorf("invalid token")
	}
	return c.UserID, nil
}
