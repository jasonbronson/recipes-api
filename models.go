package main

type Recipe struct {
	Category     string   `json:"category"`
	CookTime     int      `json:"cookTime"`
	Date         string   `json:"date"`
	Image        string   `json:"image"`
	Ingredients  []string `json:"ingredients"`
	ParsedIngredients []IngredientDetail `json:"parsedIngredients,omitempty"`
	Instructions []string `json:"instructions"`
	PrepTime     int      `json:"prepTime"`
	Servings     int      `json:"servings"`
	OriginalServings int  `json:"originalServings,omitempty"`
	Title        string   `json:"title"`
	TotalTime    int      `json:"totalTime"`
	Link         string   `json:"link"`
	OriginalURL  string   `json:"originalURL"`
	Note         *string  `json:"note,omitempty"`
	IsFavorite   bool     `json:"isFavorite"`
}

type IngredientDetail struct {
	BaseAmountValue *float64 `json:"-"`
	BaseAmountText  string   `json:"-"`
	AmountValue     *float64 `json:"amountValue,omitempty"`
	AmountText      string   `json:"amountText,omitempty"`
	Description     string   `json:"description"`
	Display         string   `json:"display"`
}
