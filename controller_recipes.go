package main

import (
	"database/sql"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func handleSaveRecipe(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var request struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	if linked, slug, err := recipeRepo.LinkRecipeIfExists(username, request.URL); err != nil {
		log.Printf("Failed to link existing recipe for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save recipe"})
		return
	} else if linked {
		recipeCache.Delete(singleRecipeCacheKey(username, slug))
		invalidateUserRecipeCaches(username)
		c.JSON(http.StatusAccepted, gin.H{"message": "recipe saved successfully"})
		return
	}

	if err := recipeRepo.EnqueueRecipe(username, request.URL); err != nil {
		log.Printf("Failed to enqueue recipe for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue recipe"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "recipe queued for processing"})
}

func handleUpsertRecipeNote(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	var request struct {
		Note string `json:"note"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "note is required"})
		return
	}

	if err := recipeRepo.UpsertRecipeNote(username, slug, request.Note); err != nil {
		log.Printf("Failed to upsert note for %s/%s: %v", username, slug, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save note"})
		return
	}

	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	c.JSON(http.StatusOK, gin.H{"message": "note saved"})
}

func handleDeleteRecipeNote(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")

	if err := recipeRepo.DeleteRecipeNote(username, slug); err != nil {
		log.Printf("Failed to delete note for %s/%s: %v", username, slug, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete note"})
		return
	}

	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	c.JSON(http.StatusOK, gin.H{"message": "note deleted"})
}

func handleFavoriteRecipe(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if err := recipeRepo.SetFavorite(username, slug, true); err != nil {
		log.Printf("Failed to favorite recipe %s/%s: %v", username, slug, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to favorite recipe"})
		return
	}

	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	c.JSON(http.StatusOK, gin.H{"message": "recipe favorited"})
}

func handleUnfavoriteRecipe(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	if err := recipeRepo.SetFavorite(username, slug, false); err != nil {
		log.Printf("Failed to unfavorite recipe %s/%s: %v", username, slug, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unfavorite recipe"})
		return
	}

	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	c.JSON(http.StatusOK, gin.H{"message": "recipe unfavorited"})
}

func handleGetRecipe(c *gin.Context) {
	slug := c.Param("name")
	username, err := usernameFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	cacheKey := singleRecipeCacheKey(username, slug)
	if cachedRecipe, found := recipeCache.Get(cacheKey); found {
		if recipe, ok := cachedRecipe.(Recipe); ok {
			log.Printf("Cache hit for %s", cacheKey)
			clone := cloneRecipe(recipe)
			scaleRecipeFromQuery(c, &clone)
			c.JSON(http.StatusOK, clone)
			return
		}
		log.Printf("Invalid cache entry for %s, evicting", cacheKey)
		recipeCache.Delete(cacheKey)
	}

	recipe, err := recipeRepo.GetRecipe(username, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
			return
		}
		log.Printf("Error fetching recipe %s for %s: %v", slug, username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch recipe"})
		return
	}

	recipeCache.Set(cacheKey, recipe, 30*time.Minute)
	clone := cloneRecipe(recipe)
	scaleRecipeFromQuery(c, &clone)
	c.JSON(http.StatusOK, clone)
}

func handleDeleteRecipe(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")

	if err := recipeRepo.DeleteRecipe(username, slug); err != nil {
		log.Printf("Error deleting recipe %s for %s: %v", slug, username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete recipe"})
		return
	}

	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	c.JSON(http.StatusOK, gin.H{"message": "recipe removed"})
}

func handleListRecipes(c *gin.Context) {
	username, err := usernameFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	category := c.Query("category")
	recipes, err := listRecipes(username, category)
	if err != nil {
		log.Printf("Error listing recipes for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list recipes"})
		return
	}

	c.JSON(http.StatusOK, recipes)
}

func handleSearchRecipes(c *gin.Context) {
	username, err := usernameFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	searchTerm := c.Query("q")
	recipes, err := recipeRepo.SearchRecipes(username, searchTerm)
	if err != nil {
		log.Printf("Error searching recipes for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to search recipes"})
		return
	}

	c.JSON(http.StatusOK, recipes)
}

func handleGetCategories(c *gin.Context) {
	username, err := usernameFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	categories, err := recipeRepo.CategoryCounts(username)
	if err != nil {
		log.Printf("Error fetching categories for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch categories"})
		return
	}

	c.JSON(http.StatusOK, categories)
}

func handleListFavorites(c *gin.Context) {
	username, err := usernameFromRequest(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	recipes, err := recipeRepo.ListFavoriteRecipes(username)
	if err != nil {
		log.Printf("Error listing favorites for %s: %v", username, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list favorites"})
		return
	}

	c.JSON(http.StatusOK, recipes)
}

func cloneRecipe(recipe Recipe) Recipe {
	clone := recipe
	if recipe.Ingredients != nil {
		clone.Ingredients = append([]string(nil), recipe.Ingredients...)
	}
	if recipe.ParsedIngredients != nil {
		clone.ParsedIngredients = append([]IngredientDetail(nil), recipe.ParsedIngredients...)
		for i := range clone.ParsedIngredients {
			if recipe.ParsedIngredients[i].BaseAmountValue != nil {
				val := *recipe.ParsedIngredients[i].BaseAmountValue
				clone.ParsedIngredients[i].BaseAmountValue = floatPtr(val)
			}
			if recipe.ParsedIngredients[i].AmountValue != nil {
				val := *recipe.ParsedIngredients[i].AmountValue
				clone.ParsedIngredients[i].AmountValue = floatPtr(val)
			}
		}
	}
	return clone
}

func scaleRecipeFromQuery(c *gin.Context, recipe *Recipe) {
	original := recipe.OriginalServings
	if original == 0 {
		recipe.OriginalServings = recipe.Servings
		original = recipe.Servings
	}

	ensureRecipeDisplays(recipe)

	scale := 1.0
	scaled := false

	if servingsStr := strings.TrimSpace(c.Query("servings")); servingsStr != "" {
		if val, err := strconv.ParseFloat(servingsStr, 64); err == nil && val > 0 && original > 0 {
			scale = val / float64(original)
			recipe.Servings = int(math.Round(val))
			scaled = true
		}
	}

	if !scaled {
		if scaleStr := strings.TrimSpace(c.Query("scale")); scaleStr != "" {
			if val, err := strconv.ParseFloat(scaleStr, 64); err == nil && val > 0 {
				scale = val
				scaled = true
				if original > 0 {
					recipe.Servings = int(math.Round(float64(original) * scale))
				}
			}
		}
	}

	if recipe.OriginalServings == 0 {
		recipe.OriginalServings = original
	}

	if scaled && scale > 0 {
		scaleParsedIngredients(recipe, scale)
	} else {
		ensureRecipeDisplays(recipe)
	}
}

func scaleParsedIngredients(recipe *Recipe, scale float64) {
	if scale <= 0 {
		scale = 1
	}

	if len(recipe.ParsedIngredients) == 0 {
		ensureRecipeDisplays(recipe)
		return
	}

	recipe.Ingredients = make([]string, len(recipe.ParsedIngredients))
	for i := range recipe.ParsedIngredients {
		detail := &recipe.ParsedIngredients[i]
		if detail.BaseAmountValue != nil {
			base := *detail.BaseAmountValue
			scaledAmount := base * scale
			detail.AmountValue = floatPtr(scaledAmount)
			detail.AmountText = formatAmount(scaledAmount)
			detail.Display = composeDisplay(detail.AmountText, detail.Description)
		} else {
			detail.AmountValue = nil
			if detail.BaseAmountText != "" {
				detail.AmountText = detail.BaseAmountText
			}
			if strings.TrimSpace(detail.Display) == "" {
				detail.Display = composeDisplay(detail.AmountText, detail.Description)
			}
		}
		recipe.Ingredients[i] = strings.TrimSpace(detail.Display)
	}
}

func ensureRecipeDisplays(recipe *Recipe) {
	if len(recipe.ParsedIngredients) == 0 {
		// fall back to existing string list
		return
	}

	recipe.Ingredients = make([]string, len(recipe.ParsedIngredients))
	for i := range recipe.ParsedIngredients {
		detail := &recipe.ParsedIngredients[i]
		if strings.TrimSpace(detail.Display) == "" {
			if detail.AmountText != "" {
				detail.Display = composeDisplay(detail.AmountText, detail.Description)
			} else if detail.BaseAmountText != "" {
				detail.Display = composeDisplay(detail.BaseAmountText, detail.Description)
			} else {
				detail.Display = strings.TrimSpace(detail.Description)
			}
		}
		recipe.Ingredients[i] = strings.TrimSpace(detail.Display)
	}
}
