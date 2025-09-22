package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret string
var jwtExpiry *time.Duration

func initJWTSecret() error {
	secret := os.Getenv("JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return errors.New("JWT_SECRET environment variable is not set")
	}
	jwtSecret = secret

	expRaw := strings.TrimSpace(os.Getenv("JWT_EXPIRATION"))
	if expRaw == "" {
		jwtExpiry = nil
		return nil
	}

	duration, err := time.ParseDuration(expRaw)
	if err != nil {
		return fmt.Errorf("invalid JWT_EXPIRATION: %w", err)
	}

	jwtExpiry = &duration
	return nil
}

func generateToken(username string, ttl time.Duration) (string, error) {
	if jwtSecret == "" {
		return "", errors.New("jwt secret not initialized")
	}

	claims := jwt.MapClaims{
		"sub": username,
		"iat": time.Now().Unix(),
	}

	var expiry time.Duration
	if jwtExpiry != nil {
		expiry = *jwtExpiry
	} else {
		expiry = ttl
	}

	if expiry > 0 {
		claims["exp"] = time.Now().Add(expiry).Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func parseToken(tokenString string) (string, error) {
	if jwtSecret == "" {
		return "", errors.New("jwt secret not initialized")
	}

	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}

	if !parsed.Valid {
		return "", errors.New("invalid token")
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	username, ok := claims["sub"].(string)
	if !ok || username == "" {
		return "", errors.New("invalid token subject")
	}

	return username, nil
}

func extractUsernameFromBearer(header string) (string, error) {
	if header == "" {
		return "", errors.New("authorization header is required")
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("authorization header must be in the format 'Bearer <token>'")
	}

	return parseToken(parts[1])
}
