package main

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
)

var errAuthRequired = errors.New("authorization header is required")

func usernameFromRequest(c *gin.Context) (string, error) {
	if username := c.Query("username"); username != "" {
		return username, nil
	}

	header := c.GetHeader("Authorization")
	if strings.TrimSpace(header) == "" {
		return "", errAuthRequired
	}

	username, err := extractUsernameFromBearer(header)
	if err != nil {
		return "", err
	}

	return username, nil
}
