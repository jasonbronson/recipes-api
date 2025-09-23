package main

type Recipe struct {
	ID                uint               `json:"id"`
	Category          string             `json:"category"`
	CookTime          int                `json:"cookTime"`
	Date              string             `json:"date"`
	Image             string             `json:"image"`
	Ingredients       []string           `json:"ingredients"`
	ParsedIngredients []IngredientDetail `json:"parsedIngredients,omitempty"`
	Instructions      []string           `json:"instructions"`
	PrepTime          int                `json:"prepTime"`
	Servings          int                `json:"servings"`
	OriginalServings  int                `json:"originalServings,omitempty"`
	Title             string             `json:"title"`
	TotalTime         int                `json:"totalTime"`
	Link              string             `json:"link"`
	OriginalURL       string             `json:"originalURL"`
	IsFavorite        bool               `json:"isFavorite"`
}

type IngredientDetail struct {
	BaseAmountValue *float64 `json:"-"`
	BaseAmountText  string   `json:"-"`
	AmountValue     *float64 `json:"amountValue,omitempty"`
	AmountText      string   `json:"amountText,omitempty"`
	Unit            string   `json:"unit,omitempty"`
	Description     string   `json:"description"`
	Display         string   `json:"display"`
}
