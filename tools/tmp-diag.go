//go:build ignore

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()
	pool, _ := pgxpool.New(ctx, "user=postgres password=postgres dbname=postgres sslmode=disable")
	defer pool.Close()

	estID := "est_01589f9176311747"
	pos := 0
	rows, _ := pool.Query(ctx, `
SELECT id, sort_order, code, calc_status, left(calc_error,80), left(raw_text,60)
FROM app_estimate_lines
WHERE estimate_id=$1 AND line_type='position' AND source='gsn'
ORDER BY sort_order, id`, estID)
	defer rows.Close()
	for rows.Next() {
		var id, code, status, err, raw string
		var sort int
		_ = rows.Scan(&id, &sort, &code, &status, &err, &raw)
		pos++
		if pos >= 32 && pos <= 36 {
			fmt.Printf("pos=%d id=%s sort=%d status=%s code=%s\n  raw=%s\n  err=%s\n", pos, id, sort, status, code, raw, err)
		}
	}
}
