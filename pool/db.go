package pool

import (
    "database/sql"
    _ "github.com/mattn/go-sqlite3"
)

func InitDB(filepath string) (*sql.DB, error) {
    db, err := sql.Open("sqlite3", filepath)
    if err != nil {
        return nil, err
    }

    // Initialize tables
    statement, err := db.Prepare("
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        address TEXT NOT NULL,
        stake FLOAT NOT NULL,
        active INTEGER NOT NULL
    )
    ")
    if err != nil {
        return nil, err
    }
    statement.Exec()

    return db, nil
}
