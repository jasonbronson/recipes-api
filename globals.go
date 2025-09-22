package main

import "github.com/patrickmn/go-cache"

var (
	recipeCache  *cache.Cache
	recipesCache *cache.Cache
	recipeRepo   *RecipeRepository
)
