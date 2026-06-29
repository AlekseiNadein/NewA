//go:build ignore

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	pool, err := pgxpool.New(context.Background(), "user=postgres password=postgres dbname=postgres sslmode=disable")
	if err != nil {
		panic(err)
	}
	defer pool.Close()

	codes := []string{"Е0624-004-06", "Е0624-004-06 (РМ59092РМ60193)", "Е0624-001-05 (РМ59092РМ60193)"}
	for _, want := range codes {
		var code, orig, name string
		err = pool.QueryRow(context.Background(), `
			SELECT code, original_code, left(name, 60)
			FROM gsn.records
			WHERE code = $1 OR original_code = $1
			LIMIT 1
		`, want).Scan(&code, &orig, &name)
		if err != nil {
			fmt.Printf("%s: %v\n", want, err)
			continue
		}
		fmt.Printf("lookup=%q -> code=%q original_code=%q name=%q\n", want, code, orig, name)
	}
}
