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
		log.Printf("Save recipe auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	var request struct {
		URL string `json:"url" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		log.Printf("Save recipe JSON binding error: %v", err)
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

func handleFavoriteRecipe(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	if idStr := strings.TrimSpace(c.Param("id")); idStr != "" {
		id64, convErr := strconv.ParseUint(idStr, 10, 64)
		if convErr != nil || id64 == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if err := recipeRepo.SetFavoriteByID(username, uint(id64), true); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
				return
			}
			log.Printf("Failed to favorite recipe %s id=%d: %v", username, id64, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to favorite recipe"})
			return
		}
		invalidateUserRecipeCaches(username)
		c.JSON(http.StatusOK, gin.H{"message": "recipe favorited"})
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

	if idStr := strings.TrimSpace(c.Param("id")); idStr != "" {
		id64, convErr := strconv.ParseUint(idStr, 10, 64)
		if convErr != nil || id64 == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if err := recipeRepo.SetFavoriteByID(username, uint(id64), false); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
				return
			}
			log.Printf("Failed to unfavorite recipe %s id=%d: %v", username, id64, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unfavorite recipe"})
			return
		}
		invalidateUserRecipeCaches(username)
		c.JSON(http.StatusOK, gin.H{"message": "recipe unfavorited"})
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
	username, err := usernameFromRequest(c)
	if err != nil {
		log.Printf("Get recipe auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	if idStr := strings.TrimSpace(c.Query("id")); idStr != "" {
		id64, convErr := strconv.ParseUint(idStr, 10, 64)
		if convErr != nil || id64 == 0 {
			log.Printf("Get recipe invalid ID error: %v, id: %s", convErr, idStr)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		cacheKey := singleRecipeIDCacheKey(username, uint(id64))
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

		recipe, err := recipeRepo.GetRecipeByID(username, uint(id64))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				log.Printf("Recipe not found for id=%d, user=%s", id64, username)
				c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
				return
			}
			log.Printf("Error fetching recipe id=%d for user=%s: %v", id64, username, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch recipe"})
			return
		}

		recipeCache.Set(cacheKey, recipe, 30*time.Minute)
		clone := cloneRecipe(recipe)
		scaleRecipeFromQuery(c, &clone)
		c.JSON(http.StatusOK, clone)
		return
	}

	// Fallback to slug
	slug := c.Param("name")
	if strings.TrimSpace(slug) == "" {
		log.Printf("Get recipe missing ID/slug error for user=%s", username)
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
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
			log.Printf("Recipe not found for slug=%s, user=%s", slug, username)
			c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
			return
		}
		log.Printf("Error fetching recipe slug=%s for user=%s: %v", slug, username, err)
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
		log.Printf("Delete recipe auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	if idStr := strings.TrimSpace(c.Param("id")); idStr != "" {
		id64, convErr := strconv.ParseUint(idStr, 10, 64)
		if convErr != nil || id64 == 0 {
			log.Printf("Delete recipe invalid ID error: %v, id: %s", convErr, idStr)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		if err := recipeRepo.DeleteRecipeByID(username, uint(id64)); err != nil {
			log.Printf("Error deleting recipe id=%d for %s: %v", id64, username, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete recipe"})
			return
		}
		invalidateUserRecipeCaches(username)
		c.JSON(http.StatusOK, gin.H{"message": "recipe removed"})
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

func handlePatchRecipe(c *gin.Context) {
	username, err := extractUsernameFromBearer(c.GetHeader("Authorization"))
	if err != nil {
		log.Printf("Patch recipe auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	slug := c.Param("slug")
	idStr := strings.TrimSpace(c.Param("id"))

	var request struct {
		Title        *string   `json:"title"`
		Instructions *[]string `json:"instructions"`
		Category     *string   `json:"category"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		log.Printf("Patch recipe JSON binding error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json body"})
		return
	}

	if request.Title == nil && request.Instructions == nil && request.Category == nil {
		log.Printf("Patch recipe no fields error for user=%s", username)
		c.JSON(http.StatusBadRequest, gin.H{"error": "no fields to update"})
		return
	}

	if idStr != "" {
		id64, convErr := strconv.ParseUint(idStr, 10, 64)
		if convErr != nil || id64 == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}
		updated, err := recipeRepo.UpdateRecipeTitleAndInstructionsByID(username, uint(id64), request.Title, request.Instructions, request.Category)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
				return
			}
			if errors.Is(err, ErrInvalidCategory) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category; allowed: breakfast, dinner, baking, other"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update recipe"})
			return
		}
		invalidateUserRecipeCaches(username)
		c.JSON(http.StatusOK, updated)
		return
	}

	updated, err := recipeRepo.UpdateRecipeTitleAndInstructions(username, slug, request.Title, request.Instructions, request.Category)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "recipe not found"})
			return
		}
		if errors.Is(err, ErrInvalidCategory) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid category; allowed: breakfast, dinner, baking, other"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update recipe"})
		return
	}

	// Invalidate caches for this user and recipe
	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	c.JSON(http.StatusOK, updated)
}

func handleListRecipes(c *gin.Context) {
	username, err := usernameFromRequest(c)
	if err != nil {
		log.Printf("List recipes auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	category := c.Query("category")
	refresh := strings.EqualFold(strings.TrimSpace(c.Query("refresh")), "true")
	recipes, err := listRecipes(username, category, refresh)
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
		log.Printf("Search recipes auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
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
		log.Printf("Get categories auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
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
		log.Printf("List favorites auth error: %v, Header: %s", err, c.GetHeader("Authorization"))
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
			detail.Display = composeDisplayWithUnit(detail.AmountText, detail.Unit, detail.Description)
		} else {
			detail.AmountValue = nil
			if detail.BaseAmountText != "" {
				detail.AmountText = detail.BaseAmountText
			}
			if strings.TrimSpace(detail.Display) == "" {
				detail.Display = composeDisplayWithUnit(detail.AmountText, detail.Unit, detail.Description)
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
				detail.Display = composeDisplayWithUnit(detail.AmountText, detail.Unit, detail.Description)
			} else if detail.BaseAmountText != "" {
				detail.Display = composeDisplayWithUnit(detail.BaseAmountText, detail.Unit, detail.Description)
			} else {
				detail.Display = strings.TrimSpace(detail.Description)
			}
		}
		recipe.Ingredients[i] = strings.TrimSpace(detail.Display)
	}
}
