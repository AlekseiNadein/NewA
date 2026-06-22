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

const (
	defaultSetID   = "alrosa-2026-q2"
	defaultSetName = "ФГИС ЦС Алроса II квартал 2026г."
	defaultPrices  = `D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-05-26 ФГИС ЦС IIкв 2026 Алроса\Prices.txt`
	defaultIndexes = `D:\OneDrive\Bases\Разработка\000 - RU ГСН-2022\2026-05-26 ФГИС ЦС IIкв 2026 Алроса\Indexes.txt`
)

func main() {
	databaseURL := os.Getenv("APP_GSN_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "user=postgres password=postgres dbname=postgres sslmode=disable"
	}

	setID := defaultSetID
	setName := defaultSetName
	pricesPath := defaultPrices
	indexesPath := defaultIndexes

	switch len(os.Args) {
	case 5:
		indexesPath = os.Args[4]
		fallthrough
	case 4:
		pricesPath = os.Args[3]
		fallthrough
	case 3:
		setName = os.Args[2]
		fallthrough
	case 2:
		setID = os.Args[1]
	case 1:
	default:
		fmt.Println("usage: import_fgis_cs [set_id] [set_name] [prices_path] [indexes_path]")
		os.Exit(1)
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Println("cwd:", err)
		os.Exit(1)
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

	service, err := gsn.NewService(databaseURL)
	if err != nil {
		fmt.Println("service:", err)
		os.Exit(1)
	}
	defer service.Close()

	count, err := service.ImportFGISSet(ctx, setID, setName, pricesPath, indexesPath)
	if err != nil {
		fmt.Println("import:", err)
		os.Exit(1)
	}

	fmt.Printf("Imported %d rows into fgis_cs.set_rows for set %q (%s).\n", count, setName, setID)
}
