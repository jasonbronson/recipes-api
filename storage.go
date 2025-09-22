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
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

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
	Slug         string    `gorm:"column:slug;not null;size:255;uniqueIndex"`
	Title        string    `gorm:"column:title;not null"`
	Category     string    `gorm:"column:category"`
	CookTime     int       `gorm:"column:cook_time"`
	Date         string    `gorm:"column:date"`
	Image        string    `gorm:"column:image"`
	Instructions string    `gorm:"column:instructions;not null"`
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

type UserRecipeModel struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"column:user_id;not null;index"`
	RecipeID  uint      `gorm:"column:recipe_id;not null;index"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserRecipeModel) TableName() string {
	return "user_recipes"
}

type NoteModel struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"column:user_id;not null;index"`
	RecipeID  uint      `gorm:"column:recipe_id;not null;index"`
	Content   string    `gorm:"column:content;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (NoteModel) TableName() string {
	return "notes"
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
	ID          uint      `gorm:"primaryKey"`
	RecipeID    uint      `gorm:"column:recipe_id;not null;index"`
	Position    int       `gorm:"column:position"`
	AmountValue *float64  `gorm:"column:amount_value"`
	AmountText  string    `gorm:"column:amount_text"`
	Description string    `gorm:"column:description"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (IngredientModel) TableName() string { return "recipe_ingredients" }

func recipeIsComplete(recipe Recipe) bool {
	if strings.TrimSpace(recipe.Title) == "" {
		return false
	}
	if len(recipe.Ingredients) == 0 || len(recipe.Instructions) == 0 {
		return false
	}

	ingValid := false
	for _, ing := range recipe.Ingredients {
		if strings.TrimSpace(ing) != "" {
			ingValid = true
			break
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

	user := UserModel{
		Username:     username,
		PasswordHash: &hashStr,
	}

	if err := r.db.Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("create user: username already exists")
		}
		return fmt.Errorf("create user: %w", err)
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

func (r *RecipeRepository) linkUserToRecipe(userID, recipeID uint) error {
	return r.linkUserToRecipeTx(r.db, userID, recipeID)
}

func (r *RecipeRepository) linkUserToRecipeTx(tx *gorm.DB, userID, recipeID uint) error {
	if userID == 0 || recipeID == 0 {
		return errors.New("user and recipe are required")
	}

	link := UserRecipeModel{UserID: userID, RecipeID: recipeID}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "recipe_id"}},
		DoNothing: true,
	}).Create(&link).Error; err != nil {
		return fmt.Errorf("link recipe to user: %w", err)
	}

	return nil
}

func (r *RecipeRepository) LinkRecipeIfExists(username, recipeURL string) (bool, string, error) {
	if strings.TrimSpace(recipeURL) == "" {
		return false, "", errors.New("url is required")
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return false, "", err
	}

	model, err := r.findRecipeByOriginalURL(recipeURL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, "", nil
		}
		return false, "", err
	}

	recipe, err := model.toRecipe()
	if err != nil {
		return false, "", err
	}

	ingDisplays, parsedIngredients, err := r.getRecipeIngredients(model.ID)
	if err != nil {
		return false, "", err
	}
	recipe.Ingredients = ingDisplays
	recipe.ParsedIngredients = parsedIngredients

	if !recipeIsComplete(recipe) {
		log.Printf("existing recipe %s lacks complete data; reprocessing", model.Slug)
		return false, "", nil
	}

	if err := r.linkUserToRecipe(userID, model.ID); err != nil {
		return false, "", err
	}

	return true, model.Slug, nil
}

func (r *RecipeRepository) getUserNote(userID, recipeID uint) (*string, error) {
	var note NoteModel
	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, recipeID).
		First(&note).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if isNoSuchTableError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get note: %w", err)
	}

	return &note.Content, nil
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

func (r *RecipeRepository) UpsertRecipeNote(username, slug, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return r.DeleteRecipeNote(username, slug)
	}

	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}

	recipeID, err := r.getRecipeIDBySlug(slug)
	if err != nil {
		return err
	}

	note := NoteModel{
		UserID:   userID,
		RecipeID: recipeID,
		Content:  content,
	}

	if err := r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "recipe_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"content":    content,
			"updated_at": time.Now(),
		}),
	}).Create(&note).Error; err != nil {
		return fmt.Errorf("upsert note: %w", err)
	}

	return nil
}

func (r *RecipeRepository) DeleteRecipeNote(username, slug string) error {
	userID, err := r.getUserID(username)
	if err != nil {
		return err
	}

	recipeID, err := r.getRecipeIDBySlug(slug)
	if err != nil {
		return err
	}

	if err := r.db.Where("user_id = ? AND recipe_id = ?", userID, recipeID).
		Delete(&NoteModel{}).Error; err != nil {
		return fmt.Errorf("delete note: %w", err)
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
		Slug:         slug,
		Title:        recipe.Title,
		Category:     recipe.Category,
		CookTime:     recipe.CookTime,
		Date:         recipe.Date,
		Image:        recipe.Image,
		Instructions: string(instructionsBytes),
		PrepTime:     recipe.PrepTime,
		Servings:     recipe.Servings,
		TotalTime:    recipe.TotalTime,
		Link:         recipe.Link,
		OriginalURL:  recipe.OriginalURL,
	}

	assignments := clause.Assignments(map[string]any{
		"title":        recipe.Title,
		"category":     recipe.Category,
		"cook_time":    recipe.CookTime,
		"date":         recipe.Date,
		"image":        recipe.Image,
		"instructions": string(instructionsBytes),
		"prep_time":    recipe.PrepTime,
		"servings":     recipe.Servings,
		"total_time":   recipe.TotalTime,
		"link":         recipe.Link,
		"original_url": recipe.OriginalURL,
		"updated_at":   gorm.Expr("CURRENT_TIMESTAMP"),
	})

	if err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "slug"}},
		DoUpdates: assignments,
	}).Create(&model).Error; err != nil {
		return fmt.Errorf("save recipe: %w", err)
	}

	if model.ID == 0 {
		if err = tx.Where("slug = ?", slug).First(&model).Error; err != nil {
			return fmt.Errorf("fetch recipe id: %w", err)
		}
	}

	if err = r.storeRecipeIngredientsTx(tx, model.ID, recipe.Ingredients); err != nil {
		return err
	}

	if err = r.linkUserToRecipeTx(tx, userID, model.ID); err != nil {
		return err
	}

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
	if err := r.db.Table("recipes").
		Select("recipes.*").
		Joins("JOIN user_recipes ur ON ur.recipe_id = recipes.id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where("u.username = ? AND recipes.slug = ?", username, slug).
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Recipe{}, sql.ErrNoRows
		}
		return Recipe{}, fmt.Errorf("get recipe: %w", err)
	}

	recipe, err := model.toRecipe()
	if err != nil {
		return Recipe{}, err
	}

	notePtr, noteErr := r.getUserNote(userID, model.ID)
	if noteErr != nil {
		return Recipe{}, noteErr
	}
	if notePtr != nil {
		recipe.Note = notePtr
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

	query := r.db.Table("recipes").
		Select("recipes.*").
		Joins("JOIN user_recipes ur ON ur.recipe_id = recipes.id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where("u.username = ?", username)

	if category != "" {
		query = query.Where("recipes.category = ?", category)
	}

	var models []RecipeModel
	if err := query.Order("ur.created_at DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}

	recipes := make([]Recipe, 0, len(models))
	for _, model := range models {
		recipe, err := model.toRecipe()
		if err != nil {
			return nil, err
		}
		notePtr, noteErr := r.getUserNote(userID, model.ID)
		if noteErr != nil {
			return nil, noteErr
		}
		if notePtr != nil {
			recipe.Note = notePtr
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
		Joins("JOIN user_recipes ur ON ur.recipe_id = recipes.id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where("u.username = ?", username).
		Where("LOWER(recipes.title) LIKE ?", likeTerm).
		Order("ur.created_at DESC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("search recipes: %w", err)
	}

	recipes := make([]Recipe, 0, len(models))
	for _, model := range models {
		recipe, err := model.toRecipe()
		if err != nil {
			return nil, err
		}
		notePtr, noteErr := r.getUserNote(userID, model.ID)
		if noteErr != nil {
			return nil, noteErr
		}
		if notePtr != nil {
			recipe.Note = notePtr
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
		Joins("JOIN users u ON u.id = f.user_id").
		Where("u.username = ?", username).
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

		ingredientDisplays, parsedIngredients, err := r.getRecipeIngredients(model.ID)
		if err != nil {
			return nil, err
		}
		recipe.Ingredients = ingredientDisplays
		recipe.ParsedIngredients = parsedIngredients

		notePtr, noteErr := r.getUserNote(userID, model.ID)
		if noteErr != nil {
			return nil, noteErr
		}
		if notePtr != nil {
			recipe.Note = notePtr
		}
		recipe.IsFavorite = true
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

func convertIngredientModel(m IngredientModel) IngredientDetail {
	detail := IngredientDetail{
		Description:    strings.TrimSpace(m.Description),
		BaseAmountText: strings.TrimSpace(m.AmountText),
	}

	if m.AmountValue != nil {
		base := *m.AmountValue
		detail.BaseAmountValue = floatPtr(base)
		detail.AmountValue = floatPtr(base)
		detail.AmountText = formatAmount(base)
	} else if detail.BaseAmountText != "" {
		detail.AmountText = detail.BaseAmountText
	}

	if detail.AmountText == "" {
		detail.Display = strings.TrimSpace(detail.Description)
	} else {
		detail.Display = composeDisplay(detail.AmountText, detail.Description)
	}

	if detail.Display == "" && detail.BaseAmountText != "" {
		detail.Display = strings.TrimSpace(detail.BaseAmountText)
	}

	return detail
}

func (r *RecipeRepository) storeRecipeIngredientsTx(tx *gorm.DB, recipeID uint, raw []string) error {
	if err := tx.Where("recipe_id = ?", recipeID).Delete(&IngredientModel{}).Error; err != nil {
		if isNoSuchTableError(err) {
			return nil
		}
		return fmt.Errorf("clear ingredients: %w", err)
	}

	if len(raw) == 0 {
		return nil
	}

	for idx, line := range raw {
		amountValue, amountText, description := parseIngredientString(line)
		if strings.TrimSpace(description) == "" && (amountValue == nil || *amountValue == 0) {
			continue
		}
		// Normalize amount text to ASCII fractions when we parsed a numeric value
		normalizedAmountText := amountText
		if amountValue != nil {
			normalizedAmountText = formatAmount(*amountValue)
		}
		model := IngredientModel{
			RecipeID:    recipeID,
			Position:    idx,
			AmountText:  normalizedAmountText,
			Description: description,
		}
		if amountValue != nil {
			model.AmountValue = amountValue
		}

		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("insert ingredient: %w", err)
		}
	}

	return nil
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

func containsNumeric(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	for _, r := range token {
		if unicode.IsDigit(r) || r == '/' || r == '.' {
			return true
		}
		if isUnicodeFraction(r) {
			return true
		}
	}
	return false
}

func isUnicodeFraction(r rune) bool {
	switch r {
	case '¼', '½', '¾', '⅐', '⅑', '⅒', '⅓', '⅔', '⅕', '⅖', '⅗', '⅘', '⅙', '⅚', '⅛', '⅜', '⅝', '⅞':
		return true
	default:
		return false
	}
}

func parseAmountTokens(tokens []string) (float64, bool) {
	if len(tokens) == 0 {
		return 0, false
	}

	if len(tokens) == 1 {
		return parseSingleToken(tokens[0])
	}

	if len(tokens) == 2 {
		first, ok := parseSingleToken(tokens[0])
		if !ok {
			return 0, false
		}
		second, ok := parseSingleToken(tokens[1])
		if ok && strings.Contains(tokens[1], "/") {
			return first + second, true
		}
		if ok && strings.ContainsRune(tokens[1], '.') {
			return first + second, true
		}
		if ok && isUnicodeFraction([]rune(tokens[1])[0]) {
			return first + second, true
		}
		return first, true
	}

	// If more than two tokens, only parse the first value.
	return parseSingleToken(tokens[0])
}

func parseSingleToken(token string) (float64, bool) {
	normalized := strings.Trim(token, ",()")
	if normalized == "" {
		return 0, false
	}
	// Handle hyphenated mixed fractions like "1-1/2"
	if hyphenMixedRe := regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*-\s*([0-9]+)\s*/\s*([0-9]+)\s*$`); hyphenMixedRe.MatchString(normalized) {
		matches := hyphenMixedRe.FindStringSubmatch(normalized)
		if len(matches) == 4 {
			whole, err1 := strconv.ParseFloat(matches[1], 64)
			num, err2 := strconv.ParseFloat(matches[2], 64)
			den, err3 := strconv.ParseFloat(matches[3], 64)
			if err1 == nil && err2 == nil && err3 == nil && den != 0 {
				return whole + (num / den), true
			}
		}
	}
	// Handle tokens that combine a whole number with a unicode fraction, e.g., "1½", "2¾"
	// We scan the token to extract any leading numeric part and a unicode fraction rune.
	{
		var fractionRune rune
		var wholePartBuilder strings.Builder
		for _, r := range normalized {
			if isUnicodeFraction(r) {
				fractionRune = r
				continue
			}
			if unicode.IsDigit(r) || r == '.' {
				wholePartBuilder.WriteRune(r)
				continue
			}
			// Ignore simple delimiters occasionally embedded with the amount
			if r == ' ' || r == '-' || r == '_' {
				continue
			}
			// Non-numeric character found; keep scanning in case a fraction rune appears later
		}
		if fractionRune != 0 {
			if frac, ok := unicodeFractionToFloat(fractionRune); ok {
				whole := 0.0
				if wholePartBuilder.Len() > 0 {
					if w, err := strconv.ParseFloat(wholePartBuilder.String(), 64); err == nil {
						whole = w
					}
				}
				return whole + frac, true
			}
		}
	}

	if len(normalized) == 1 && isUnicodeFraction([]rune(normalized)[0]) {
		return unicodeFractionToFloat([]rune(normalized)[0])
	}

	if strings.Contains(normalized, "/") {
		parts := strings.Split(normalized, "/")
		if len(parts) != 2 {
			return 0, false
		}
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 != nil || err2 != nil || den == 0 {
			return 0, false
		}
		return num / den, true
	}

	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func unicodeFractionToFloat(r rune) (float64, bool) {
	switch r {
	case '¼':
		return 0.25, true
	case '½':
		return 0.5, true
	case '¾':
		return 0.75, true
	case '⅓':
		return 1.0 / 3.0, true
	case '⅔':
		return 2.0 / 3.0, true
	case '⅕':
		return 0.2, true
	case '⅖':
		return 0.4, true
	case '⅗':
		return 0.6, true
	case '⅘':
		return 0.8, true
	case '⅙':
		return 1.0 / 6.0, true
	case '⅚':
		return 5.0 / 6.0, true
	case '⅛':
		return 0.125, true
	case '⅜':
		return 0.375, true
	case '⅝':
		return 0.625, true
	case '⅞':
		return 0.875, true
	default:
		return 0, false
	}
}

func (r *RecipeRepository) DeleteRecipe(username, slug string) error {
	if username == "" {
		return errors.New("username is required")
	}

	if err := r.db.Where("user_id = (SELECT id FROM users WHERE username = ?) AND recipe_id = (SELECT id FROM recipes WHERE slug = ?)", username, slug).
		Delete(&UserRecipeModel{}).Error; err != nil {
		return fmt.Errorf("delete recipe link: %w", err)
	}

	return nil
}

func (r *RecipeRepository) CountRecipes(username string) (int64, error) {
	if username == "" {
		return 0, errors.New("username is required")
	}

	var count int64
	if err := r.db.Model(&UserRecipeModel{}).
		Joins("JOIN users ON users.id = user_recipes.user_id").
		Where("users.username = ?", username).
		Count(&count).Error; err != nil {
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
		Joins("JOIN user_recipes ur ON ur.recipe_id = recipes.id").
		Joins("JOIN users u ON u.id = ur.user_id").
		Where("u.username = ?", username).
		Group("recipes.category").
		Order("LOWER(recipes.category)").
		Scan(&results).Error; err != nil {
		return nil, fmt.Errorf("category counts: %w", err)
	}

	return results, nil
}

func (r *RecipeRepository) getRecipeIngredients(recipeID uint) ([]string, []IngredientDetail, error) {
	var rows []IngredientModel
	if err := r.db.Where("recipe_id = ?", recipeID).Order("position ASC").Find(&rows).Error; err != nil {
		if isNoSuchTableError(err) {
			return []string{}, []IngredientDetail{}, nil
		}
		return nil, nil, fmt.Errorf("fetch ingredients: %w", err)
	}

	displays := make([]string, 0, len(rows))
	details := make([]IngredientDetail, 0, len(rows))
	for _, row := range rows {
		detail := convertIngredientModel(row)
		details = append(details, detail)
		displays = append(displays, detail.Display)
	}

	return displays, details, nil
}

func (m RecipeModel) toRecipe() (Recipe, error) {
	var recipe Recipe

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

	return recipe, nil
}
