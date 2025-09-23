CREATE TABLE IF NOT EXISTS recipes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    slug TEXT NOT NULL,
    title TEXT NOT NULL,
    category TEXT,
    cook_time INTEGER,
    date TEXT,
    image TEXT,
    instructions TEXT NOT NULL,
    ingredients TEXT,
    parsed_ingredients TEXT,
    prep_time INTEGER,
    servings INTEGER,
    total_time INTEGER,
    link TEXT,
    original_url TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, slug),
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_recipes_category ON recipes(category);
