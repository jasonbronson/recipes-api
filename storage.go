package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RecipeRepository struct {
	db *gorm.DB
}

type UserModel struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"column:username;uniqueIndex;size:255;not null"`
	PasswordHash *string   `gorm:"column:password_hash"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (UserModel) TableName() string {
	return "users"
}

type RecipeModel struct {
	ID           uint      `gorm:"primaryKey"`
	UserID       uint      `gorm:"column:user_id;not null;index;uniqueIndex:uid_slug"`
	Slug         string    `gorm:"column:slug;not null;size:255;uniqueIndex:uid_slug"`
	Title        string    `gorm:"column:title;not null"`
	Category     string    `gorm:"column:category"`
	CookTime     int       `gorm:"column:cook_time"`
	Date         string    `gorm:"column:date"`
	Image        string    `gorm:"column:image"`
	Instructions string    `gorm:"column:instructions;not null"`
	Ingredients  string    `gorm:"column:ingredients"`
	ParsedJSON   string    `gorm:"column:parsed_ingredients"`
	PrepTime     int       `gorm:"column:prep_time"`
	Servings     int       `gorm:"column:servings"`
	TotalTime    int       `gorm:"column:total_time"`
	Link         string    `gorm:"column:link"`
	OriginalURL  string    `gorm:"column:original_url"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (RecipeModel) TableName() string {
	return "recipes"
}

type QueueModel struct {
	ID          uint       `gorm:"primaryKey"`
	UserID      uint       `gorm:"column:user_id;index;not null"`
	User        UserModel  `gorm:"foreignKey:UserID"`
	URL         string     `gorm:"column:url;not null"`
	Attempts    int        `gorm:"column:attempts"`
	LastError   *string    `gorm:"column:last_error"`
	ProcessedAt *time.Time `gorm:"column:processed_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (QueueModel) TableName() string {
	return "queue"
}

type CategoryCount struct {
	Category string
	Count    int64
}

type UserProfile struct {
	Username  string
	CreatedAt time.Time
}

type FavoriteModel struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"column:user_id;not null;index"`
	RecipeID  uint      `gorm:"column:recipe_id;not null;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (FavoriteModel) TableName() string {
	return "favorites"
}

func isNoSuchTableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}

func floatPtr(v float64) *float64 {
	value := v
	return &value
}

func composeDisplay(amountText, description string) string {
	amount := strings.TrimSpace(amountText)
	desc := strings.TrimSpace(description)
	if amount == "" {
		return desc
	}
	if desc == "" {
		return amount
	}
	return strings.TrimSpace(amount + " " + desc)
}

type IngredientModel struct {
}

var (
	allowedCategories = map[string]struct{}{
		"breakfast": {},
		"dinner":    {},
		"baking":    {},
		"other":     {},
	}
	ErrInvalidCategory = errors.New("invalid category")
)

func normalizeCategoryOrOther(category string) string {
	c := strings.ToLower(strings.TrimSpace(category))
	if _, ok := allowedCategories[c]; ok {
		return c
	}
	return "other"
}

func normalizeCategoryStrict(category string) (string, bool) {
	c := strings.ToLower(strings.TrimSpace(category))
	if _, ok := allowedCategories[c]; ok {
		return c, true
	}
	return "", false
}

func recipeIsComplete(recipe Recipe) bool {
	if strings.TrimSpace(recipe.Title) == "" {
		return false
	}
	if len(recipe.Ingredients) == 0 && len(recipe.ParsedIngredients) == 0 {
		return false
	}
	if len(recipe.Instructions) == 0 {
		return false
	}

	ingValid := false
	// Check raw ingredients list
	for _, ing := range recipe.Ingredients {
		if strings.TrimSpace(ing) != "" {
			ingValid = true
			break
		}
	}
	// If none valid, check parsed ingredients
	if !ingValid {
		for _, d := range recipe.ParsedIngredients {
			if strings.TrimSpace(d.Description) != "" || strings.TrimSpace(d.AmountText) != "" || d.AmountValue != nil {
				ingValid = true
				break
			}
		}
	}

	insValid := false
	for _, step := range recipe.Instructions {
		if strings.TrimSpace(step) != "" {
			insValid = true
			break
		}
	}

	return ingValid && insValid
}

func formatAmount(value float64) string {
	if value <= 0 {
		return ""
	}

	whole := math.Floor(value)
	frac := value - whole

	if frac < 1e-6 {
		return strconv.FormatFloat(whole, 'f', 0, 64)
	}

	denominators := []int{2, 3, 4, 8, 16}
	bestNum := 0
	bestDen := 1
	minDiff := math.MaxFloat64

	for _, den := range denominators {
		num := int(math.Round(frac * float64(den)))
		diff := math.Abs(frac - float64(num)/float64(den))
		if diff < minDiff {
			minDiff = diff
			bestNum = num
			bestDen = den
		}
	}

	if bestNum == 0 {
		return strconv.FormatFloat(value, 'f', 2, 64)
	}

	g := gcd(bestNum, bestDen)
	bestNum /= g
	bestDen /= g

	if whole < 1e-6 {
		return fmt.Sprintf("%d/%d", bestNum, bestDen)
	}

	return fmt.Sprintf("%d %d/%d", int(whole), bestNum, bestDen)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

type PasswordResetModel struct {
	ID        uint       `gorm:"primaryKey"`
	UserID    uint       `gorm:"column:user_id;index;not null"`
	User      UserModel  `gorm:"foreignKey:UserID"`
	TokenHash string     `gorm:"column:token_hash;uniqueIndex;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (PasswordResetModel) TableName() string {
	return "password_resets"
}

func NewRecipeRepository(db *gorm.DB) *RecipeRepository {
	return &RecipeRepository{db: db}
}

func (r *RecipeRepository) getUserID(username string) (uint, error) {
	if username == "" {
		return 0, errors.New("username is required")
	}

	var user UserModel
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, sql.ErrNoRows
		}
		return 0, fmt.Errorf("lookup user: %w", err)
	}

	return user.ID, nil
}

func (r *RecipeRepository) CreateUser(username, password string) error {
	if username == "" || password == "" {
		return errors.New("username and password are required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	hashStr := string(hash)

	// Create user and assign default recipes in a single transaction
	tx := r.db.Begin()
	if err := tx.Error; err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	user := UserModel{
		Username:     username,
		PasswordHash: &hashStr,
	}
	if err = tx.Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("create user: username already exists")
		}
		return fmt.Errorf("create user: %w", err)
	}

	// Assign the first 4 recipes (by oldest created) to the new user (copy to user-owned)
	var defaultRecipes []RecipeModel
	if err = tx.Order("created_at ASC").Limit(4).Find(&defaultRecipes).Error; err != nil {
		// Do not fail registration if recipes table missing; commit user creation
		if isNoSuchTableError(err) {
			log.Println("[Registration] recipes table missing; skipping default recipe assignment")
			return nil
		}
		return fmt.Errorf("fetch default recipes: %w", err)
	}

	for _, rec := range defaultRecipes {
		// Create a user-owned copy with same slug (if conflict, append suffix)
		copy := rec
		copy.ID = 0
		copy.UserID = user.ID
		// Ensure unique (user_id, slug)
		trySlug := copy.Slug
		for attempt := 0; attempt < 3; attempt++ {
			copy.Slug = trySlug
			if err := tx.Create(&copy).Error; err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") {
					trySlug = fmt.Sprintf("%s-%d", rec.Slug, attempt+2)
					continue
				}
				return err
			}
			break
		}
	}

	if len(defaultRecipes) > 0 {
		log.Printf("[Registration] assigned %d default recipes to user %s", len(defaultRecipes), username)
	} else {
		log.Printf("[Registration] no recipes found to assign to user %s", username)
	}

	return nil
}

func (r *RecipeRepository) AuthenticateUser(username, password string) (uint, error) {
	if username == "" || password == "" {
		return 0, errors.New("username and password are required")
	}

	var user UserModel
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errors.New("invalid credentials")
		}
		return 0, fmt.Errorf("lookup user: %w", err)
	}

	if user.PasswordHash == nil {
		return 0, errors.New("password not set")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return 0, errors.New("invalid credentials")
	}

	return user.ID, nil
}

func (r *RecipeRepository) GetUserProfile(username string) (UserProfile, error) {
	if username == "" {
		return UserProfile{}, errors.New("username is required")
	}

	var user UserModel
	if err := r.db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return UserProfile{}, sql.ErrNoRows
		}
		return UserProfile{}, fmt.Errorf("lookup user: %w", err)
	}

	return UserProfile{Username: user.Username, CreatedAt: user.CreatedAt}, nil
}

func (r *RecipeRepository) findRecipeByOriginalURL(originalURL string) (*RecipeModel, error) {
	if strings.TrimSpace(originalURL) == "" {
		return nil, errors.New("original url is required")
	}

	var model RecipeModel
	if err := r.db.Where("original_url = ?", originalURL).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("find recipe: %w", err)
	}

	return &model, nil
}

func (r *RecipeRepository) getRecipeIDBySlug(slug string) (uint, error) {
	if strings.TrimSpace(slug) == "" {
		return 0, errors.New("slug is required")
	}

	var model RecipeModel
	if err := r.db.Where("slug = ?", slug).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, sql.ErrNoRows
		}
		return 0, fmt.Errorf("lookup recipe: %w", err)
	}

	return model.ID, nil
}

func (r *RecipeRepository) LinkRecipeIfExists(username, recipeURL string) (bool, string, error) {
	if strings.TrimSpace(recipeURL) == "" {
		return false, "", errors.New("url is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return false, "", err
	}

	var model *RecipeModel
	if err := r.db.Where("user_id = ? AND original_url = ?", userID, recipeURL).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("find recipe: %w", err)
	}

	recipe, err := model.toRecipe()
	if err != nil {
		return false, "", err
	}

	// Populate ingredients from JSON on model
	if strings.TrimSpace(model.Ingredients) != "" {
		_ = json.Unmarshal([]byte(model.Ingredients), &recipe.Ingredients)
	}
	if strings.TrimSpace(model.ParsedJSON) != "" {
		_ = json.Unmarshal([]byte(model.ParsedJSON), &recipe.ParsedIngredients)
	}

	if !recipeIsComplete(recipe) {
		log.Printf("existing recipe %s lacks complete data; reprocessing", model.Slug)
		return false, "", nil
	}

	return true, model.Slug, nil
}

func (r *RecipeRepository) isFavorite(userID, recipeID uint) (bool, error) {
	var fav FavoriteModel
	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, recipeID).
		Select("id").First(&fav).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if isNoSuchTableError(err) {
			return false, nil
		}
		return false, fmt.Errorf("get favorite: %w", err)
	}
	return true, nil
}

func (r *RecipeRepository) SetFavorite(username, slug string, favorite bool) error {
	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}

	recipeID, err := r.getRecipeIDBySlug(slug)
	if err != nil {
		return err
	}

	if favorite {
		fav := FavoriteModel{UserID: userID, RecipeID: recipeID}
		if err := r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "recipe_id"}},
			DoNothing: true,
		}).Create(&fav).Error; err != nil {
			if isNoSuchTableError(err) {
				return nil
			}
			return fmt.Errorf("set favorite: %w", err)
		}
		return nil
	}

	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, recipeID).
		Delete(&FavoriteModel{}).Error; err != nil {
		if isNoSuchTableError(err) {
			return nil
		}
		return fmt.Errorf("remove favorite: %w", err)
	}

	return nil
}

func (r *RecipeRepository) CreatePasswordReset(username string, ttl time.Duration) (string, error) {
	userID, err := r.getUserID(username)
	if err != nil {
		return "", err
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	if token == "" {
		return "", errors.New("failed to generate token")
	}

	hashBytes := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hashBytes[:])

	// Invalidate previous unused resets for this user.
	if err := r.db.Model(&PasswordResetModel{}).
		Where("user_id = ? AND used_at IS NULL", userID).
		Updates(map[string]any{
			"used_at":    time.Now(),
			"updated_at": time.Now(),
		}).Error; err != nil {
		return "", fmt.Errorf("invalidate resets: %w", err)
	}

	reset := PasswordResetModel{
		UserID:    userID,
		TokenHash: hashHex,
		ExpiresAt: time.Now().Add(ttl),
	}

	if err := r.db.Create(&reset).Error; err != nil {
		return "", fmt.Errorf("create reset token: %w", err)
	}

	return token, nil
}

func (r *RecipeRepository) ResetPasswordWithToken(token, newPassword string) error {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(newPassword) == "" {
		return errors.New("token and password are required")
	}

	hash := sha256.Sum256([]byte(token))
	hashHex := hex.EncodeToString(hash[:])

	var reset PasswordResetModel
	if err := r.db.Preload("User").
		Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", hashHex, time.Now()).
		First(&reset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("invalid or expired token")
		}
		return fmt.Errorf("lookup reset token: %w", err)
	}

	if err := r.updateUserPassword(reset.UserID, newPassword); err != nil {
		return err
	}

	now := time.Now()
	if err := r.db.Model(&PasswordResetModel{}).
		Where("id = ?", reset.ID).
		Updates(map[string]any{
			"used_at":    now,
			"updated_at": now,
		}).Error; err != nil {
		return fmt.Errorf("mark token used: %w", err)
	}

	return nil
}

// UpdateRecipeTitleAndInstructions updates only the title and/or instructions
// for a recipe identified by slug, limited to recipes linked to the username.
// If both fields are empty/nil, it is a no-op. Returns the updated recipe.
func (r *RecipeRepository) UpdateRecipeTitleAndInstructions(username, slug string, title *string, instructions *[]string, category *string) (Recipe, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(slug) == "" {
		return Recipe{}, errors.New("username and slug are required")
	}

	// Ensure user has access and get recipe model
	var model RecipeModel
	if err := r.db.Table("recipes").
		Select("recipes.*").
		Joins("JOIN users u ON u.id = recipes.user_id").
		Where("u.username = ? AND recipes.slug = ?", username, slug).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Recipe{}, sql.ErrNoRows
		}
		return Recipe{}, fmt.Errorf("get recipe for update: %w", err)
	}

	updates := map[string]any{
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if title != nil {
		updates["title"] = strings.TrimSpace(*title)
	}
	if instructions != nil {
		// Marshal instructions array to JSON string as stored in DB
		data, err := json.Marshal(*instructions)
		if err != nil {
			return Recipe{}, fmt.Errorf("marshal instructions: %w", err)
		}
		updates["instructions"] = string(data)
	}
	if category != nil {
		if norm, ok := normalizeCategoryStrict(*category); ok {
			updates["category"] = norm
		} else {
			return Recipe{}, ErrInvalidCategory
		}
	}

	if len(updates) > 1 { // more than just updated_at
		if err := r.db.Model(&RecipeModel{}).Where("id = ?", model.ID).Updates(updates).Error; err != nil {
			return Recipe{}, fmt.Errorf("update recipe: %w", err)
		}
	}

	// Re-fetch and return updated recipe
	var refreshed RecipeModel
	if err := r.db.First(&refreshed, model.ID).Error; err != nil {
		return Recipe{}, fmt.Errorf("reload recipe: %w", err)
	}
	recipe, err := refreshed.toRecipe()
	if err != nil {
		return Recipe{}, err
	}
	return recipe, nil
}

// UpdateRecipeTitleAndInstructionsByID updates title and/or instructions by recipe ID for the given user
func (r *RecipeRepository) UpdateRecipeTitleAndInstructionsByID(username string, recipeID uint, title *string, instructions *[]string, category *string) (Recipe, error) {
	if strings.TrimSpace(username) == "" || recipeID == 0 {
		return Recipe{}, errors.New("username and id are required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return Recipe{}, err
	}

	// Verify ownership/access
	var model RecipeModel
	if err := r.db.Table("recipes").
		Select("recipes.*").
		Joins("JOIN users u ON u.id = recipes.user_id").
		Where("u.id = ? AND recipes.id = ?", userID, recipeID).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Recipe{}, sql.ErrNoRows
		}
		return Recipe{}, fmt.Errorf("get recipe for update: %w", err)
	}

	updates := map[string]any{
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if title != nil {
		updates["title"] = strings.TrimSpace(*title)
	}
	if instructions != nil {
		data, err := json.Marshal(*instructions)
		if err != nil {
			return Recipe{}, fmt.Errorf("marshal instructions: %w", err)
		}
		updates["instructions"] = string(data)
	}
	if category != nil {
		if norm, ok := normalizeCategoryStrict(*category); ok {
			updates["category"] = norm
		} else {
			return Recipe{}, ErrInvalidCategory
		}
	}
	if len(updates) > 1 {
		if err := r.db.Model(&RecipeModel{}).Where("id = ?", model.ID).Updates(updates).Error; err != nil {
			return Recipe{}, fmt.Errorf("update recipe: %w", err)
		}
	}

	var refreshed RecipeModel
	if err := r.db.First(&refreshed, model.ID).Error; err != nil {
		return Recipe{}, fmt.Errorf("reload recipe: %w", err)
	}
	recipe, err := refreshed.toRecipe()
	if err != nil {
		return Recipe{}, err
	}
	return recipe, nil
}

func (r *RecipeRepository) updateUserPassword(userID uint, newPassword string) error {
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("password is required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := r.db.Model(&UserModel{}).Where("id = ?", userID).
		Update("password_hash", string(hash)).Error; err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	return nil
}

func (r *RecipeRepository) EnqueueRecipe(username, recipeURL string) error {
	if strings.TrimSpace(recipeURL) == "" {
		return errors.New("url is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}

	var existing QueueModel
	if err := r.db.Where("user_id = ? AND url = ? AND processed_at IS NULL", userID, recipeURL).
		First(&existing).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check pending queue item: %w", err)
	}

	item := QueueModel{
		UserID: userID,
		URL:    recipeURL,
	}

	if err := r.db.Create(&item).Error; err != nil {
		return fmt.Errorf("enqueue recipe: %w", err)
	}

	return nil
}

func (r *RecipeRepository) FetchPendingQueue(limit int) ([]QueueModel, error) {
	query := r.db.Preload("User").
		Where("processed_at IS NULL").
		Order("created_at ASC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var items []QueueModel
	if err := query.Find(&items).Error; err != nil {
		return nil, fmt.Errorf("fetch queue: %w", err)
	}

	return items, nil
}

func (r *RecipeRepository) MarkQueueItemResult(id uint, processErr error) error {
	updates := map[string]any{
		"attempts":   gorm.Expr("attempts + 1"),
		"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
	}

	if processErr == nil {
		updates["processed_at"] = gorm.Expr("CURRENT_TIMESTAMP")
		updates["last_error"] = nil
	} else {
		msg := processErr.Error()
		if len(msg) > 1024 {
			msg = msg[:1024]
		}
		updates["last_error"] = msg
	}

	if err := r.db.Model(&QueueModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update queue item: %w", err)
	}

	if processErr != nil {
		var item QueueModel
		if err := r.db.First(&item, id).Error; err == nil {
			if item.Attempts >= 5 && item.ProcessedAt == nil {
				if err := r.db.Model(&QueueModel{}).
					Where("id = ?", id).
					Update("processed_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
					return fmt.Errorf("finalize queue item: %w", err)
				}
			}
		}
	}

	return nil
}

func (r *RecipeRepository) SaveRecipeForUser(username, slug string, recipe Recipe) (err error) {
	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}

	instructionsBytes, err := json.Marshal(recipe.Instructions)
	if err != nil {
		return fmt.Errorf("marshal instructions: %w", err)
	}
	ingredientsBytes, err := json.Marshal(recipe.Ingredients)
	if err != nil {
		return fmt.Errorf("marshal ingredients: %w", err)
	}
	parsedBytes, err := json.Marshal(recipe.ParsedIngredients)
	if err != nil {
		return fmt.Errorf("marshal parsed ingredients: %w", err)
	}

	tx := r.db.Begin()
	if err := tx.Error; err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	model := RecipeModel{
		UserID:       userID,
		Slug:         slug,
		Title:        recipe.Title,
		Category:     normalizeCategoryOrOther(recipe.Category),
		CookTime:     recipe.CookTime,
		Date:         recipe.Date,
		Image:        recipe.Image,
		Instructions: string(instructionsBytes),
		Ingredients:  string(ingredientsBytes),
		ParsedJSON:   string(parsedBytes),
		PrepTime:     recipe.PrepTime,
		Servings:     recipe.Servings,
		TotalTime:    recipe.TotalTime,
		Link:         recipe.Link,
		OriginalURL:  recipe.OriginalURL,
	}

	assignments := clause.Assignments(map[string]any{
		"title":              recipe.Title,
		"category":           normalizeCategoryOrOther(recipe.Category),
		"cook_time":          recipe.CookTime,
		"date":               recipe.Date,
		"image":              recipe.Image,
		"instructions":       string(instructionsBytes),
		"ingredients":        string(ingredientsBytes),
		"parsed_ingredients": string(parsedBytes),
		"prep_time":          recipe.PrepTime,
		"servings":           recipe.Servings,
		"total_time":         recipe.TotalTime,
		"link":               recipe.Link,
		"original_url":       recipe.OriginalURL,
		"updated_at":         gorm.Expr("CURRENT_TIMESTAMP"),
	})

	if err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "slug"}},
		DoUpdates: assignments,
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("save recipe: %w", err)
	}

	if model.ID == 0 {
		if err = tx.Where("user_id = ? AND slug = ?", userID, slug).First(&model).Error; err != nil {
			return fmt.Errorf("fetch recipe id: %w", err)
		}
	}

	// Ingredients now persisted on the recipe row as JSON

	// Legacy user_recipes link omitted in user-owned model

	return nil
}

func (r *RecipeRepository) GetRecipe(username, slug string) (Recipe, error) {
	if username == "" {
		return Recipe{}, errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return Recipe{}, err
	}

	var model RecipeModel
	if err := r.db.Where("user_id = ? AND slug = ?", userID, slug).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Recipe{}, sql.ErrNoRows
		}
		return Recipe{}, fmt.Errorf("get recipe: %w", err)
	}

	recipe, err := model.toRecipe()
	if err != nil {
		return Recipe{}, err
	}

	if fav, favErr := r.isFavorite(userID, model.ID); favErr == nil {
		recipe.IsFavorite = fav
	} else {
		return Recipe{}, favErr
	}

	return recipe, nil
}

func (r *RecipeRepository) ListRecipes(username, category string) ([]Recipe, error) {
	if username == "" {
		return nil, errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return nil, err
	}

	var models []RecipeModel
	if err := r.db.Table("recipes").
		Select("recipes.*").
		Where("recipes.user_id = ?", userID).
		Order("recipes.created_at DESC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}

	recipes := make([]Recipe, 0, len(models))
	for _, model := range models {
		recipe, err := model.toRecipe()
		if err != nil {
			return nil, err
		}

		if fav, favErr := r.isFavorite(userID, model.ID); favErr != nil {
			return nil, favErr
		} else {
			recipe.IsFavorite = fav
		}
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

func (r *RecipeRepository) SearchRecipes(username, term string) ([]Recipe, error) {
	if username == "" {
		return nil, errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return nil, err
	}

	likeTerm := fmt.Sprintf("%%%s%%", strings.ToLower(term))

	var models []RecipeModel
	if err := r.db.Table("recipes").
		Select("recipes.*").
		Where("recipes.user_id = ?", userID).
		Where("LOWER(recipes.title) LIKE ?", likeTerm).
		Order("recipes.created_at DESC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("search recipes: %w", err)
	}

	recipes := make([]Recipe, 0, len(models))
	for _, model := range models {
		recipe, err := model.toRecipe()
		if err != nil {
			return nil, err
		}

		if fav, favErr := r.isFavorite(userID, model.ID); favErr != nil {
			return nil, favErr
		} else {
			recipe.IsFavorite = fav
		}
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

func (r *RecipeRepository) ListFavoriteRecipes(username string) ([]Recipe, error) {
	if username == "" {
		return nil, errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return nil, err
	}

	var models []RecipeModel
	if err := r.db.Table("recipes").
		Select("recipes.*").
		Joins("JOIN favorites f ON f.recipe_id = recipes.id").
		Where("f.user_id = ? AND recipes.user_id = ?", userID, userID).
		Order("f.created_at DESC").
		Find(&models).Error; err != nil {
		if isNoSuchTableError(err) {
			return []Recipe{}, nil
		}
		return nil, fmt.Errorf("list favorites: %w", err)
	}

	recipes := make([]Recipe, 0, len(models))
	for _, model := range models {
		recipe, err := model.toRecipe()
		if err != nil {
			return nil, err
		}
		recipe.OriginalServings = recipe.Servings

		// Populate ingredients from JSON columns
		if strings.TrimSpace(model.Ingredients) != "" {
			_ = json.Unmarshal([]byte(model.Ingredients), &recipe.Ingredients)
		}
		if strings.TrimSpace(model.ParsedJSON) != "" {
			_ = json.Unmarshal([]byte(model.ParsedJSON), &recipe.ParsedIngredients)
		}
		recipe.IsFavorite = true
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

func (r *RecipeRepository) GetRecipeByID(username string, recipeID uint) (Recipe, error) {
	if username == "" {
		return Recipe{}, errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return Recipe{}, err
	}

	var model RecipeModel
	if err := r.db.Where("id = ? AND user_id = ?", recipeID, userID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Recipe{}, sql.ErrNoRows
		}
		return Recipe{}, fmt.Errorf("get recipe: %w", err)
	}

	recipe, err := model.toRecipe()
	if err != nil {
		return Recipe{}, err
	}

	if fav, favErr := r.isFavorite(userID, model.ID); favErr == nil {
		recipe.IsFavorite = fav
	} else {
		return Recipe{}, favErr
	}

	return recipe, nil
}

func (r *RecipeRepository) SetFavoriteByID(username string, recipeID uint, favorite bool) error {
	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}

	// Ensure the recipe belongs to the user (owned or linked)
	var cnt int64
	if err := r.db.Model(&RecipeModel{}).Where("id = ? AND user_id = ?", recipeID, userID).Count(&cnt).Error; err != nil {
		return fmt.Errorf("check ownership: %w", err)
	}
	if cnt == 0 {
		return sql.ErrNoRows
	}

	if favorite {
		fav := FavoriteModel{UserID: userID, RecipeID: recipeID}
		if err := r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "recipe_id"}},
			DoNothing: true,
		}).Create(&fav).Error; err != nil {
			if isNoSuchTableError(err) {
				return nil
			}
			return fmt.Errorf("set favorite: %w", err)
		}
		return nil
	}

	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, recipeID).
		Delete(&FavoriteModel{}).Error; err != nil {
		if isNoSuchTableError(err) {
			return nil
		}
		return fmt.Errorf("remove favorite: %w", err)
	}

	return nil
}

func (r *RecipeRepository) DeleteRecipeByID(username string, recipeID uint) error {
	if username == "" {
		return errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}
	var model RecipeModel
	if err := r.db.Where("user_id = ? AND id = ?", userID, recipeID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("lookup recipe: %w", err)
	}
	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, model.ID).Delete(&FavoriteModel{}).Error; err != nil {
		if !isNoSuchTableError(err) {
			return fmt.Errorf("delete favorites: %w", err)
		}
	}
	if err := r.db.Delete(&RecipeModel{}, model.ID).Error; err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	return nil
}

// composeDisplayWithUnit builds a display string from amount, unit, and description.
// It preserves existing behavior when fields are empty, and inserts spaces appropriately.
func composeDisplayWithUnit(amountText, unit, description string) string {
	amount := strings.TrimSpace(amountText)
	unit = strings.TrimSpace(unit)
	desc := strings.TrimSpace(description)

	parts := make([]string, 0, 3)
	if amount != "" {
		parts = append(parts, amount)
	}
	if unit != "" {
		parts = append(parts, unit)
	}
	if desc != "" {
		parts = append(parts, desc)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// extractUnitFromDescription attempts to parse a unit token from the start of the description.
// Returns the unit (lowercased, as encountered) and the remaining description without the unit and optional "of".
func extractUnitFromDescription(description string) (string, string) {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return "", ""
	}

	// Tokenize description
	fields := strings.Fields(desc)
	if len(fields) == 0 {
		return "", desc
	}

	// Known units (one and two-word variants)
	oneWord := map[string]struct{}{
		"tsp": {}, "teaspoon": {}, "teaspoons": {},
		"tbsp": {}, "tablespoon": {}, "tablespoons": {},
		"cup": {}, "cups": {}, "c": {},
		"pint": {}, "pints": {}, "pt": {},
		"quart": {}, "quarts": {}, "qt": {},
		"gallon": {}, "gallons": {}, "gal": {},
		"ml": {}, "milliliter": {}, "milliliters": {},
		"l": {}, "liter": {}, "liters": {},
		"oz": {}, "ounce": {}, "ounces": {},
		"lb": {}, "lbs": {}, "pound": {}, "pounds": {},
		"g": {}, "gram": {}, "grams": {},
		"kg": {}, "kilogram": {}, "kilograms": {},
		"pinch": {}, "dash": {},
		"clove": {}, "cloves": {},
		"can": {}, "cans": {},
		"package": {}, "packages": {},
		"stick": {}, "sticks": {},
	}

	twoWord := map[string]struct{}{
		"fl oz": {}, "fluid ounce": {}, "fluid ounces": {},
	}

	// Attempt two-word unit first
	if len(fields) >= 2 {
		candidate := strings.ToLower(strings.TrimSpace(fields[0] + " " + fields[1]))
		if _, ok := twoWord[candidate]; ok {
			unit := candidate
			remain := strings.TrimSpace(strings.Join(fields[2:], " "))
			if strings.HasPrefix(strings.ToLower(remain), "of ") {
				remain = strings.TrimSpace(remain[3:])
			}
			return unit, remain
		}
	}

	// Fallback to one-word unit
	first := strings.ToLower(fields[0])
	if _, ok := oneWord[first]; ok {
		unit := fields[0]
		remain := strings.TrimSpace(strings.Join(fields[1:], " "))
		if strings.HasPrefix(strings.ToLower(remain), "of ") {
			remain = strings.TrimSpace(remain[3:])
		}
		return strings.ToLower(unit), remain
	}

	return "", desc
}

func parseIngredientString(input string) (*float64, string, string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, "", ""
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil, "", trimmed
	}

	amountTokens := make([]string, 0, len(fields))
	idx := 0
	for idx < len(fields) {
		token := strings.Trim(fields[idx], ",()")
		if token == "" {
			idx++
			continue
		}
		if token == "-" && len(amountTokens) > 0 {
			break
		}
		if containsNumeric(token) {
			amountTokens = append(amountTokens, token)
			idx++
			continue
		}
		break
	}

	if len(amountTokens) == 0 {
		return nil, "", trimmed
	}

	amountStr := strings.Join(amountTokens, " ")
	remaining := strings.Join(fields[idx:], " ")
	remaining = strings.TrimSpace(remaining)

	if val, ok := parseAmountTokens(amountTokens); ok {
		return floatPtr(val), amountStr, remaining
	}

	return nil, "", trimmed
}

// Deprecated parsing helpers retained for potential future use
func containsNumeric(token string) bool { return false }

func isUnicodeFraction(r rune) bool { return false }

func parseAmountTokens(tokens []string) (float64, bool) { return 0, false }

func parseSingleToken(token string) (float64, bool) { return 0, false }

func unicodeFractionToFloat(r rune) (float64, bool) { return 0, false }

func (r *RecipeRepository) DeleteRecipe(username, slug string) error {
	if username == "" {
		return errors.New("username is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}
	// Find recipe id first
	var model RecipeModel
	if err := r.db.Where("user_id = ? AND slug = ?", userID, slug).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("lookup recipe: %w", err)
	}
	// Delete favorites and notes scoped to this user and recipe
	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, model.ID).Delete(&FavoriteModel{}).Error; err != nil {
		if !isNoSuchTableError(err) {
			return fmt.Errorf("delete favorites: %w", err)
		}
	}

	// Delete recipe row (ingredients cascade via FK in SQL)
	if err := r.db.Delete(&RecipeModel{}, model.ID).Error; err != nil {
		return fmt.Errorf("delete recipe: %w", err)
	}
	return nil
}

func (r *RecipeRepository) CountRecipes(username string) (int64, error) {
	if username == "" {
		return 0, errors.New("username is required")
	}

	var count int64
	if err := r.db.Table("recipes").
		Select("COUNT(*)").
		Where("user_id = (SELECT id FROM users WHERE username = ?)", username).
		Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count recipes: %w", err)
	}

	return count, nil
}

func (r *RecipeRepository) CategoryCounts(username string) ([]CategoryCount, error) {
	if username == "" {
		return nil, errors.New("username is required")
	}

	var results []CategoryCount
	if err := r.db.Table("recipes").
		Select("COALESCE(recipes.category, '') AS category, COUNT(*) AS count").
		Joins("JOIN users u ON u.id = recipes.user_id").
		Where("u.username = ?", username).
		Group("recipes.category").
		Order("LOWER(recipes.category)").
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("category counts: %w", err)
	}

	return results, nil
}

func (m RecipeModel) toRecipe() (Recipe, error) {
	var recipe Recipe

	recipe.ID = m.ID
	recipe.Category = m.Category
	recipe.CookTime = m.CookTime
	recipe.Date = m.Date
	recipe.Image = m.Image
	recipe.PrepTime = m.PrepTime
	recipe.Servings = m.Servings
	recipe.Title = m.Title
	recipe.TotalTime = m.TotalTime
	recipe.Link = m.Link
	recipe.OriginalURL = m.OriginalURL

	if len(m.Instructions) > 0 {
		if err := json.Unmarshal([]byte(m.Instructions), &recipe.Instructions); err != nil {
			return Recipe{}, fmt.Errorf("unmarshal instructions: %w", err)
		}
	}
	if strings.TrimSpace(m.Ingredients) != "" {
		if err := json.Unmarshal([]byte(m.Ingredients), &recipe.Ingredients); err != nil {
			return Recipe{}, fmt.Errorf("unmarshal ingredients: %w", err)
		}
	}
	if strings.TrimSpace(m.ParsedJSON) != "" {
		if err := json.Unmarshal([]byte(m.ParsedJSON), &recipe.ParsedIngredients); err != nil {
			return Recipe{}, fmt.Errorf("unmarshal parsed ingredients: %w", err)
		}
	}

	return recipe, nil
}
