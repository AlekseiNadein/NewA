package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"nav-saas-mvp/backend/internal/gsn"

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

	sourcePath := filepath.Join(repoRoot, "Регионы.txt")
	if len(os.Args) > 1 {
		sourcePath = os.Args[1]
	}

	schemaPath := filepath.Join(repoRoot, "db", "gsn_schema.sql")
	schemaSQL, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Println("read schema:", err)
		os.Exit(1)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, string(schemaSQL)); err != nil {
		fmt.Println("apply schema:", err)
		os.Exit(1)
	}

	rows, err := gsn.ParseRegionsFile(sourcePath)
	if err != nil {
		fmt.Println("parse:", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Println("no regions found in", sourcePath)
		os.Exit(1)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Println("begin:", err)
		os.Exit(1)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `TRUNCATE gsn.regions`); err != nil {
		fmt.Println("truncate:", err)
		os.Exit(1)
	}

	for _, row := range rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO gsn.regions (code, name, line_no, raw_line)
			VALUES ($1, $2, $3, $4)
		`, row.Code, row.Name, row.LineNo, row.RawLine)
		if err != nil {
			fmt.Println("insert:", err)
			os.Exit(1)
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Println("commit:", err)
		os.Exit(1)
	}

	fmt.Printf("Imported %d regions from %s into gsn.regions.\n", len(rows), sourcePath)
}
