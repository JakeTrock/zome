package db

import (
	"database/sql"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

const Birth = `
-- Table for groups
CREATE TABLE Groups (
    id INTEGER PRIMARY KEY,
    group_name TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Table for storing users
CREATE TABLE Users (
    id INTEGER PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_username ON Users(username);

-- Join table for users and groups
CREATE TABLE UserGroups (
    user_id INTEGER,
    group_id INTEGER,
    PRIMARY KEY (user_id, group_id),
    FOREIGN KEY (user_id) REFERENCES Users(id),
    FOREIGN KEY (group_id) REFERENCES Groups(id)
);

CREATE INDEX idx_user_id ON UserGroups(id);

-- Table for storing messages
CREATE TABLE Messages (
    id INTEGER PRIMARY KEY,
    sender_id INTEGER NOT NULL,
    receiver_id INTEGER NOT NULL, -- NULL for broadcast messages, can be a group_id or user_id
    payload TEXT NOT NULL,
    timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP, -- clients can purge this based on age rather than by read
    FOREIGN KEY (sender_id) REFERENCES Users(id),
    FOREIGN KEY (receiver_id) REFERENCES Users(id)
);

-- Index on receiver_id for efficient querying
CREATE INDEX idx_receiver_id ON Messages(receiver_id);

-- Table for storing files
CREATE TABLE Files (
    id INTEGER PRIMARY KEY,
    owner_id INTEGER NOT NULL,
    group_id INTEGER, -- NULL for private files
    ACL INTEGER NOT NULL, -- typical UNIX-style permissions
    filename TEXT NOT NULL,
    directory_id INTEGER NOT NULL,
    length INTEGER NOT NULL,
    hash TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expiry TIMESTAMP, -- NULL for no expiry
    FOREIGN KEY (owner_id) REFERENCES Users(id)
);

-- Table for storing directories
CREATE TABLE Directories (
    id INTEGER PRIMARY KEY,
    owner_id INTEGER NOT NULL,
    group_id INTEGER, -- NULL for private directories
    ACL INTEGER NOT NULL, -- typical UNIX-style permissions
    parent_directory_id INTEGER, -- NULL for root directory
    directory_name TEXT NOT NULL
);

-- Table for storing blocks
CREATE TABLE Blocks (
    id INTEGER PRIMARY KEY,
    part_id INTEGER NOT NULL,
    block_order INTEGER NOT NULL, -- Order of the block in the chain
    block_data BLOB NOT NULL,
    FOREIGN KEY (part_id) REFERENCES Files(id)
);

-- Indexes for efficient partitioning
CREATE INDEX idx_block_owner ON Blocks(owner_id);
CREATE INDEX idx_block_id ON Blocks(block_id);
	`

func InitTables(dbPath string, createTable bool) (*sql.Tx, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if createTable {
		_, err = db.Exec(Birth)
		if err != nil {
			return nil, err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// CreateDbIfNotExists checks if the database file exists, and creates it if it does not, returning a boolean indicating whether the database file existed or nots
func CreateDbIfNotExists(dbPath string) (bool, error) {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		_, err := os.Create(dbPath)
		if err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

const Death = `
	DROP TABLE IF EXISTS Groups;
	DROP TABLE IF EXISTS Users;
	DROP TABLE IF EXISTS UserGroups;
	DROP TABLE IF EXISTS Messages;
	DROP TABLE IF EXISTS Files;
	DROP TABLE IF EXISTS Directories;
	DROP TABLE IF EXISTS Blocks;
	`

func ClearTables(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	_, err = db.Exec(Death)
	if err != nil {
		return err
	}
	db.Close()
	return nil
}

func QueryTable(dbInst *sql.Tx, query string) (*sql.Rows, error) {
	// AND (%s)
	//SHADDUP
	return dbInst.Query(query)
}
