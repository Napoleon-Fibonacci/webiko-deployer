package main

import (
	"database/sql"
	"errors"
	"os"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// One database seam: everything touches Postgres through getDB().
var (
	dbMu     sync.Mutex
	dbHandle *sql.DB
)

func getDB() (*sql.DB, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	if dbHandle != nil {
		return dbHandle, nil
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL tidak diset. Hubungkan Neon di Vercel.")
	}
	handle, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	handle.SetMaxOpenConns(5)
	handle.SetMaxIdleConns(2)
	handle.SetConnMaxLifetime(30 * time.Minute)
	dbHandle = handle
	return dbHandle, nil
}
