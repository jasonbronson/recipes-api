package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/patrickmn/go-cache"
	ginprometheus "github.com/zsais/go-gin-prometheus"
)

func main() {
	recipeCache = cache.New(30*24*time.Hour, 1*time.Hour)
	recipesCache = cache.New(1*time.Hour, 10*time.Minute)

	db, err := InitDatabase()
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to access database handle: %v", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()

	recipeRepo = NewRecipeRepository(db)

	if err := godotenv.Load(); err != nil {
		log.Println("Info: No .env file found, using environment variables only")
	}

	if err := initJWTSecret(); err != nil {
		log.Fatalf("failed to load JWT secret: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runQueueProcessor(ctx, recipeRepo)

	router := gin.Default()
	attachMiddleware(router)
	registerRoutes(router)

	// Get port from environment variable, default to 8080 for local development
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s", port)
	router.Run(":" + port)
}

func attachMiddleware(router *gin.Engine) {
	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	p := ginprometheus.NewPrometheus("gin")
	p.Use(router)
}

func registerRoutes(router *gin.Engine) {
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "Pong"})
	})

	router.POST("/register", handleRegister)
	router.POST("/login", handleLogin)
	router.POST("/password-reset/request", handlePasswordResetRequest)
	router.POST("/password-reset/confirm", handlePasswordResetConfirm)
	router.GET("/profile", handleGetProfile)

	router.POST("/save-recipe", handleSaveRecipe)
	router.GET("/get-recipe/:name", handleGetRecipe)
	router.DELETE("/recipes/:slug", handleDeleteRecipe)

	// edit recipes
	router.DELETE("/recipes/id/:id", handleDeleteRecipe)
	router.PATCH("/recipes/id/:id", handlePatchRecipe)

	// edit favorites
	router.POST("/recipes/id/:id/favorite", handleFavoriteRecipe)
	router.DELETE("/recipes/id/:id/favorite", handleUnfavoriteRecipe)

	router.GET("/get-recipes", handleListRecipes)
	router.GET("/search-recipes", handleSearchRecipes)
	router.GET("/categories", handleGetCategories)
	router.GET("/favorites", handleListFavorites)
}
