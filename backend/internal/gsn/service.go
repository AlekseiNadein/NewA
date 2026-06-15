package gsn

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var ErrNotConfigured = errors.New("gsn database is not configured")

type Node struct {
	Code        string `json:"code"`
	ParentCode  string `json:"parentCode,omitempty"`
	Level       int    `json:"level"`
	Name        string `json:"name"`
	Unit        string `json:"unit,omitempty"`
	NormList    string `json:"normList,omitempty"`
	RecordCount int    `json:"recordCount"`
	HasChildren bool   `json:"hasChildren"`
}

type BaseInfo struct {
	Edition string `json:"edition"`
	Version string `json:"version"`
}

type Service struct {
	db *sql.DB
}

func NewService(databaseURL string) (*Service, error) {
	if databaseURL == "" {
		return &Service{}, nil
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)

	return &Service{db: db}, nil
}

func (s *Service) Configured() bool {
	return s != nil && s.db != nil
}

func (s *Service) Close() error {
	if !s.Configured() {
		return nil
	}
	return s.db.Close()
}

func (s *Service) ListChildren(ctx context.Context, parentCode string, limit int) ([]Node, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			h.code,
			COALESCE(h.parent_code, '') AS parent_code,
			h.level,
			h.name,
			h.unit,
			h.raw_norm_list,
			(
				SELECT count(*)
				FROM gsn.hierarchy_record_refs refs
				WHERE refs.hierarchy_code = h.code
			) AS record_count,
			EXISTS (
				SELECT 1
				FROM gsn.hierarchy child
				WHERE child.parent_code = h.code
			) AS has_children
		FROM gsn.hierarchy h
		WHERE (($1::text = '' AND h.parent_code IS NULL) OR h.parent_code = NULLIF($1::text, ''))
		ORDER BY h.line_no
		LIMIT $2
	`, parentCode, limit)
	if err != nil {
		return nil, fmt.Errorf("query gsn hierarchy: %w", err)
	}
	defer rows.Close()

	nodes := make([]Node, 0)
	for rows.Next() {
		var node Node
		if err := rows.Scan(
			&node.Code,
			&node.ParentCode,
			&node.Level,
			&node.Name,
			&node.Unit,
			&node.NormList,
			&node.RecordCount,
			&node.HasChildren,
		); err != nil {
			return nil, fmt.Errorf("scan gsn hierarchy: %w", err)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn hierarchy: %w", err)
	}

	return nodes, nil
}

func (s *Service) BaseInfo(ctx context.Context) (BaseInfo, error) {
	if !s.Configured() {
		return BaseInfo{}, ErrNotConfigured
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT param_key, param_value
		FROM gsn.base_info_params
		WHERE param_key IN ($1, $2)
	`, "Редакция СНБ", "Версия")
	if err != nil {
		return BaseInfo{}, fmt.Errorf("query gsn base info: %w", err)
	}
	defer rows.Close()

	var info BaseInfo
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return BaseInfo{}, fmt.Errorf("scan gsn base info: %w", err)
		}

		switch key {
		case "Редакция СНБ":
			info.Edition = value
		case "Версия":
			info.Version = value
		}
	}
	if err := rows.Err(); err != nil {
		return BaseInfo{}, fmt.Errorf("read gsn base info: %w", err)
	}

	return info, nil
}
