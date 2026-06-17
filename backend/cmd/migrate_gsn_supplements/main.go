package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	databaseURL := os.Getenv("APP_GSN_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "user=postgres password=postgres dbname=postgres sslmode=disable"
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Println("cwd:", err)
		os.Exit(1)
	}

	sqlPath := filepath.Join(repoRoot, "db", "gsn_migrate_supplements.sql")
	sqlBytes, err := os.ReadFile(sqlPath)
	if err != nil {
		fmt.Println("read migration:", err)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	if _, err := db.ExecContext(context.Background(), string(sqlBytes)); err != nil {
		fmt.Println("migrate:", err)
		os.Exit(1)
	}

	fmt.Println("GSN supplements migration applied.")
}
