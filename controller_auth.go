package main

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func handleRegister(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	if err := recipeRepo.CreateUser(request.Username, request.Password); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "username already exists") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "user registered"})
}

func handleLogin(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		log.Printf("Error binding JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	if _, err := recipeRepo.AuthenticateUser(request.Username, request.Password); err != nil {
		if strings.Contains(err.Error(), "invalid credentials") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		log.Printf("Error authenticating user %s: %v", request.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authenticate"})
		return
	}

	token, err := generateToken(request.Username, tokenTTL)
	if err != nil {
		log.Printf("Error generating token for %s: %v", request.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(tokenTTL.Seconds()),
	})
}

func handlePasswordResetRequest(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	token, err := recipeRepo.CreatePasswordReset(request.Username, passwordResetTTL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusAccepted, gin.H{"message": "password reset email sent if account exists"})
			return
		}
		log.Printf("Error creating password reset for %s: %v", request.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create password reset"})
		return
	}

	if err := sendPasswordResetEmail(request.Username, token); err != nil {
		log.Printf("Error sending password reset email to %s: %v", request.Username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to send password reset email"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "password reset email sent"})
}

func handlePasswordResetConfirm(c *gin.Context) {
	var request struct {
		Token    string `json:"token" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token and password are required"})
		return
	}

	if err := recipeRepo.ResetPasswordWithToken(request.Token, request.Password); err != nil {
		if strings.Contains(err.Error(), "invalid or expired token") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
			return
		}
		log.Printf("Error resetting password: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password reset successful"})
}

func handleGetProfile(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	profile, err := recipeRepo.GetUserProfile(username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		log.Printf("Error fetching profile for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"email":     profile.Username,
		"createdAt": profile.CreatedAt.UTC().Format(time.RFC3339),
	})
}
