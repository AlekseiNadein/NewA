package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nav-saas-mvp/backend/internal/gsn"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const defaultResursPath = `D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-06-15 Сокращенная база\Books\00~resurs.txt`

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

	sourcePath := defaultResursPath
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

	rows, err := gsn.ParseResourceCodifierFile(sourcePath)
	if err != nil {
		fmt.Println("parse:", err)
		os.Exit(1)
	}
	if len(rows) == 0 {
		fmt.Println("no resource codifier rows found in", sourcePath)
		os.Exit(1)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Println("begin:", err)
		os.Exit(1)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `TRUNCATE gsn.resource_codifier`); err != nil {
		fmt.Println("truncate:", err)
		os.Exit(1)
	}

	const batchSize = 500
	for start := 0; start < len(rows); start += batchSize {
		end := start + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		var builder strings.Builder
		builder.WriteString(`INSERT INTO gsn.resource_codifier (number, code, name, unit, line_no) VALUES `)
		args := make([]any, 0, len(batch)*5)
		for i, row := range batch {
			if i > 0 {
				builder.WriteString(",")
			}
			base := len(args) + 1
			fmt.Fprintf(&builder, "($%d,$%d,$%d,$%d,$%d)", base, base+1, base+2, base+3, base+4)
			args = append(args, row.Number, row.Code, row.Name, row.Unit, row.LineNo)
		}
		if _, err := tx.ExecContext(ctx, builder.String(), args...); err != nil {
			fmt.Println("insert batch:", err)
			os.Exit(1)
		}
	}

	if err := tx.Commit(); err != nil {
		fmt.Println("commit:", err)
		os.Exit(1)
	}

	fmt.Printf("Imported %d resource codifier rows from %s into gsn.resource_codifier.\n", len(rows), sourcePath)
}
