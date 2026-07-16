package gsn

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

var ErrNotConfigured = errors.New("gsn database is not configured")

type Node struct {
	Code         string `json:"code"`
	ParentCode   string `json:"parentCode,omitempty"`
	Level        int    `json:"level"`
	Name         string `json:"name"`
	Unit         string `json:"unit,omitempty"`
	NormList     string `json:"normList,omitempty"`
	OriginalCode string `json:"originalCode,omitempty"`
	NodeType     string `json:"nodeType,omitempty"`
	DocumentRef  string `json:"documentRef,omitempty"`
	DocumentFile string `json:"documentFile,omitempty"`
	RecordCount  int    `json:"recordCount"`
	HasChildren  bool   `json:"hasChildren"`
}

type Supplement struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Edition     string `json:"edition,omitempty"`
	VersionDate string `json:"versionDate,omitempty"`
	Ordinal     int    `json:"ordinal"`
}

type BaseInfo struct {
	Supplement string `json:"supplement"`
	Edition    string `json:"edition"`
	Version    string `json:"version"`
}

type Region struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type Service struct {
	db          *sql.DB
	recordCache *recordDetailCache
}

func NewService(databaseURL string) (*Service, error) {
	if databaseURL == "" {
		return &Service{}, nil
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	s := &Service{db: db}
	s.SetMaxOpenConns(16)

	return s, nil
}

func (s *Service) SetMaxOpenConns(n int) {
	if !s.Configured() {
		return
	}
	if n < 4 {
		n = 4
	}
	s.db.SetMaxOpenConns(n)
	if n < 8 {
		s.db.SetMaxIdleConns(n)
	} else {
		s.db.SetMaxIdleConns(8)
	}
	s.db.SetConnMaxLifetime(30 * time.Minute)
}

func (s *Service) Configured() bool {
	return s != nil && s.db != nil
}

func (s *Service) Close() error {
	var err error
	if s.recordCache != nil {
		if closeErr := s.recordCache.close(); closeErr != nil {
			err = closeErr
		}
	}
	if !s.Configured() {
		return err
	}
	if closeErr := s.db.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	return err
}

func (s *Service) ListSupplements(ctx context.Context) ([]Supplement, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT code, label, edition, version_date, ordinal
		FROM gsn.supplements
		ORDER BY ordinal, code
	`)
	if err != nil {
		return nil, fmt.Errorf("query gsn supplements: %w", err)
	}
	defer rows.Close()

	items := make([]Supplement, 0)
	for rows.Next() {
		var item Supplement
		if err := rows.Scan(&item.Code, &item.Label, &item.Edition, &item.VersionDate, &item.Ordinal); err != nil {
			return nil, fmt.Errorf("scan gsn supplement: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn supplements: %w", err)
	}

	return items, nil
}

func (s *Service) ListChildren(ctx context.Context, supplementCode, parentCode string, limit int) ([]Node, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	if supplementCode == "" {
		return nil, fmt.Errorf("supplement code is required")
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
			h.node_type,
			h.document_ref,
			h.document_file,
			(
				SELECT count(*)
				FROM gsn.hierarchy_record_refs refs
				WHERE refs.supplement_code = h.supplement_code
					AND refs.hierarchy_code = h.code
			) AS record_count,
			COALESCE(
				(
					SELECT COALESCE(NULLIF(rec.original_code, ''), rec.code)
					FROM gsn.hierarchy_record_refs refs
					JOIN gsn.records rec ON rec.code = refs.record_code
					WHERE refs.supplement_code = h.supplement_code
						AND refs.hierarchy_code = h.code
					ORDER BY refs.ordinal
					LIMIT 1
				),
				''
			) AS original_code,
			EXISTS (
				SELECT 1
				FROM gsn.hierarchy child
				WHERE child.supplement_code = h.supplement_code
					AND child.parent_code = h.code
			) AS has_children
		FROM gsn.hierarchy h
		WHERE h.supplement_code = $1
			AND (($2::text = '' AND h.parent_code IS NULL) OR h.parent_code = NULLIF($2::text, ''))
		ORDER BY h.line_no
		LIMIT $3
	`, supplementCode, parentCode, limit)
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
			&node.NodeType,
			&node.DocumentRef,
			&node.DocumentFile,
			&node.RecordCount,
			&node.OriginalCode,
			&node.HasChildren,
		); err != nil {
			return nil, fmt.Errorf("scan gsn hierarchy: %w", err)
		}
		if !node.HasChildren && node.OriginalCode == "" && strings.TrimSpace(node.NormList) != "" {
			refs := parseNormListRefs(node.NormList)
			if len(refs) > 0 {
				if detail, _, _, err := s.lookupRecordDetail(ctx, refs[0]); err == nil {
					node.OriginalCode = detail.OriginalCode
					if node.OriginalCode == "" {
						node.OriginalCode = detail.Code
					}
				}
			}
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn hierarchy: %w", err)
	}

	return nodes, nil
}

func (s *Service) BaseInfo(ctx context.Context, supplementCode string) (BaseInfo, error) {
	if !s.Configured() {
		return BaseInfo{}, ErrNotConfigured
	}
	if supplementCode == "" {
		return BaseInfo{}, fmt.Errorf("supplement code is required")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT bip.param_key, bip.param_value
		FROM gsn.base_info_params bip
		WHERE bip.supplement_code = $1
			AND bip.param_key IN ($2, $3)
	`, supplementCode, "Редакция СНБ", "Версия")
	if err != nil {
		return BaseInfo{}, fmt.Errorf("query gsn base info: %w", err)
	}
	defer rows.Close()

	info := BaseInfo{Supplement: supplementCode}
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

	if info.Edition == "" || info.Version == "" {
		var edition, versionDate string
		err := s.db.QueryRowContext(ctx, `
			SELECT edition, version_date
			FROM gsn.supplements
			WHERE code = $1
		`, supplementCode).Scan(&edition, &versionDate)
		if err == nil {
			if info.Edition == "" {
				info.Edition = edition
			}
			if info.Version == "" {
				info.Version = versionDate
			}
		}
	}

	return info, nil
}

func (s *Service) ListRegions(ctx context.Context) ([]Region, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT code, name
		FROM gsn.regions
		ORDER BY line_no, code
	`)
	if err != nil {
		return nil, fmt.Errorf("query gsn regions: %w", err)
	}
	defer rows.Close()

	items := make([]Region, 0)
	for rows.Next() {
		var item Region
		if err := rows.Scan(&item.Code, &item.Name); err != nil {
			return nil, fmt.Errorf("scan gsn region: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn regions: %w", err)
	}

	return items, nil
}
