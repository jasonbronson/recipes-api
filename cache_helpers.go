package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

func singleRecipeCacheKey(username, slug string) string {
	return fmt.Sprintf("recipe:%s:%s", username, slug)
}

func recipeListCacheKey(username, category string) string {
	if category == "" {
		return fmt.Sprintf("recipes:%s:all", username)
	}
	return fmt.Sprintf("recipes:%s:%s", username, category)
}

func invalidateUserRecipeCaches(username string) {
	prefix := fmt.Sprintf("recipes:%s:", username)
	for key := range recipesCache.Items() {
		if strings.HasPrefix(key, prefix) {
			recipesCache.Delete(key)
		}
	}
}

func listRecipes(username, category string) ([]Recipe, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}

	cacheKey := recipeListCacheKey(username, category)
	if cachedRecipes, found := recipesCache.Get(cacheKey); found {
		if recipes, ok := cachedRecipes.([]Recipe); ok {
			log.Printf("Cache hit for %s", cacheKey)
			return recipes, nil
		}
		log.Printf("Invalid cache entry for %s, evicting", cacheKey)
		recipesCache.Delete(cacheKey)
	}

	recipes, err := recipeRepo.ListRecipes(username, category)
	if err != nil {
		return nil, err
	}

	recipesCache.Set(cacheKey, recipes, 1*time.Hour)

	return recipes, nil
}
