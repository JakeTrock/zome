package libzome

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

func (a *App) InitDb() (*sql.DB, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dbPath := filepath.Join(homeDir, "database.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Create the table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS my_table (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			data TEXT
		)
	`)
	if err != nil {
		return nil, err
	}

	return db, nil
}

//TODO: create ORM like db functions
