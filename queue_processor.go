package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

func runQueueProcessor(ctx context.Context, repo *RecipeRepository) {
	log.Println("queue processor started")
	safeProcessQueueBatch(repo)
	ticker := time.NewTicker(queuePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("queue processor stopping")
			return
		case <-ticker.C:
			log.Println("queue processor tick")
			safeProcessQueueBatch(repo)
		}
	}
}

func safeProcessQueueBatch(repo *RecipeRepository) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("queue processor recovered from panic: %v", r)
		}
	}()

	processQueueBatch(repo)
}

func processQueueBatch(repo *RecipeRepository) {
	items, err := repo.FetchPendingQueue(queueBatchSize)
	if err != nil {
		log.Printf("Queue: fetch error: %v", err)
		return
	}

	if len(items) == 0 {
		log.Println("Queue: empty")
		return
	}

	log.Printf("Queue: processing %d item(s) with concurrency=%d", len(items), queueConcurrency)

	// Concurrency limiter
	workerSlots := make(chan struct{}, queueConcurrency)
	var wg sync.WaitGroup

	for _, item := range items {
		workerSlots <- struct{}{}
		wg.Add(1)
		go func(itm QueueModel) {
			defer func() {
				<-workerSlots
				wg.Done()
			}()
			processQueueItem(repo, itm)
		}(item)
	}

	wg.Wait()
}

func processQueueItem(repo *RecipeRepository, item QueueModel) {
	defer func() {
		if r := recover(); r != nil {
			err := fmt.Errorf("queue item %d panic: %v", item.ID, r)
			log.Println(err)
			if markErr := repo.MarkQueueItemResult(item.ID, err); markErr != nil {
				log.Printf("failed to mark queue item %d after panic: %v", item.ID, markErr)
			}
		}
	}()

	username := item.User.Username
	if username == "" {
		err := fmt.Errorf("queue item %d missing username", item.ID)
		log.Println(err)
		if markErr := repo.MarkQueueItemResult(item.ID, err); markErr != nil {
			log.Printf("failed to mark queue item %d: %v", item.ID, markErr)
		}
		return
	}

	log.Printf("Queue: processing item %d for user %s", item.ID, username)
	if linked, slug, err := repo.LinkRecipeIfExists(username, item.URL); err != nil {
		log.Printf("Queue: item %d failed linking existing recipe: %v", item.ID, err)
		if markErr := repo.MarkQueueItemResult(item.ID, err); markErr != nil {
			log.Printf("failed to mark queue item %d: %v", item.ID, markErr)
		}
		return
	} else if linked {
		recipeCache.Delete(singleRecipeCacheKey(username, slug))
		invalidateUserRecipeCaches(username)
		if err := repo.MarkQueueItemResult(item.ID, nil); err != nil {
			log.Printf("Queue: failed to finalize item %d: %v", item.ID, err)
		}
		return
	}

	recipe, slug, err := getRecipe(item.URL)
	if err != nil {
		log.Printf("Queue: item %d failed to fetch recipe: %v", item.ID, err)
		// Fallback: create a placeholder recipe so the user can see the item
		title, fallbackSlug := FallbackTitleAndSlug(item.URL)
		placeholder := Recipe{
			Title:        title,
			OriginalURL:  item.URL,
			Category:     "",
			Ingredients:  []string{},
			Instructions: []string{},
		}
		if saveErr := repo.SaveRecipeForUser(username, fallbackSlug, placeholder); saveErr != nil {
			log.Printf("Queue: item %d failed to save placeholder recipe: %v", item.ID, saveErr)
			if markErr := repo.MarkQueueItemResult(item.ID, err); markErr != nil {
				log.Printf("failed to mark queue item %d: %v", item.ID, markErr)
			}
			return
		}
		// Mark processed since we stored a placeholder successfully
		recipeCache.Delete(singleRecipeCacheKey(username, fallbackSlug))
		invalidateUserRecipeCaches(username)
		if markErr := repo.MarkQueueItemResult(item.ID, nil); markErr != nil {
			log.Printf("Queue: failed to finalize item %d after placeholder save: %v", item.ID, markErr)
		}
		return
	}
	recipe.Link = fmt.Sprintf("/recipes/%s/%s", recipe.Category, slug)

	if !recipeIsComplete(recipe) {
		// Save a minimal placeholder so the user has something (title/image/original URL)
		log.Printf("Queue: item %d recipe incomplete; saving minimal placeholder", item.ID)
		fallbackTitle, fallbackSlug := FallbackTitleAndSlug(item.URL)
		minimalSlug := slug
		if minimalSlug == "" {
			minimalSlug = fallbackSlug
		}
		minimalTitle := recipe.Title
		if minimalTitle == "" {
			minimalTitle = fallbackTitle
		}
		placeholder := Recipe{
			Title:        minimalTitle,
			Image:        recipe.Image,
			OriginalURL:  item.URL,
			Category:     recipe.Category,
			Ingredients:  []string{},
			Instructions: []string{},
		}
		if saveErr := repo.SaveRecipeForUser(username, minimalSlug, placeholder); saveErr != nil {
			log.Printf("Queue: item %d failed to save minimal placeholder: %v", item.ID, saveErr)
			if markErr := repo.MarkQueueItemResult(item.ID, saveErr); markErr != nil {
				log.Printf("failed to mark queue item %d: %v", item.ID, markErr)
			}
			return
		}
		recipeCache.Delete(singleRecipeCacheKey(username, minimalSlug))
		invalidateUserRecipeCaches(username)
		if markErr := repo.MarkQueueItemResult(item.ID, nil); markErr != nil {
			log.Printf("Queue: failed to finalize item %d after minimal placeholder save: %v", item.ID, markErr)
		}
		return
	}

	if err := repo.SaveRecipeForUser(username, slug, recipe); err != nil {
		log.Printf("Queue: item %d failed to save recipe: %v", item.ID, err)
		if markErr := repo.MarkQueueItemResult(item.ID, err); markErr != nil {
			log.Printf("failed to mark queue item %d: %v", item.ID, markErr)
		}
		return
	}

	recipeCache.Delete(singleRecipeCacheKey(username, slug))
	invalidateUserRecipeCaches(username)

	if err := repo.MarkQueueItemResult(item.ID, nil); err != nil {
		log.Printf("Queue: failed to finalize item %d: %v", item.ID, err)
	}
}
