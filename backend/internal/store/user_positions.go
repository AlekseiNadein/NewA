package store

import (
	"context"
	"errors"
	"strings"

	"nav-saas-mvp/backend/internal/domain"

	"github.com/jackc/pgx/v5"
)

func (s *FileStore) ListUserPositions(ctx context.Context, companyID string) ([]domain.UserPosition, error) {
	if s.treeDB == nil {
		return []domain.UserPosition{}, nil
	}
	rows, err := s.treeDB.Query(ctx, `
SELECT id, code, name, unit, cost
FROM app_user_positions
WHERE company_id = $1
ORDER BY sort_order, code, id
`, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.UserPosition, 0)
	for rows.Next() {
		var item domain.UserPosition
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Unit, &item.Cost); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *FileStore) ReplaceUserPositions(ctx context.Context, companyID string, items []domain.UserPosition) ([]domain.UserPosition, error) {
	if s.treeDB == nil {
		return items, nil
	}
	tx, err := s.treeDB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM app_user_positions WHERE company_id = $1`, companyID); err != nil {
		return nil, err
	}

	normalized := make([]domain.UserPosition, 0, len(items))
	seen := map[string]struct{}{}
	for i, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = newID("upos")
		}
		code := strings.TrimSpace(item.Code)
		name := strings.TrimSpace(item.Name)
		if code == "" || name == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			id = newID("upos")
		}
		seen[id] = struct{}{}
		item = domain.UserPosition{
			ID:   id,
			Code: code,
			Name: name,
			Unit: strings.TrimSpace(item.Unit),
			Cost: item.Cost,
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO app_user_positions (id, company_id, code, name, unit, cost, sort_order)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, item.ID, companyID, item.Code, item.Name, item.Unit, item.Cost, i); err != nil {
			return nil, err
		}
		normalized = append(normalized, item)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return normalized, nil
}

func (s *FileStore) LookupUserPositionByCode(ctx context.Context, companyID, code string) (domain.UserPosition, bool, error) {
	if s.treeDB == nil {
		return domain.UserPosition{}, false, nil
	}
	code = userCatalogCodeKey(code)
	if code == "" {
		return domain.UserPosition{}, false, nil
	}
	var item domain.UserPosition
	err := s.treeDB.QueryRow(ctx, `
SELECT id, code, name, unit, cost
FROM app_user_positions
WHERE company_id = $1 AND code = $2
ORDER BY sort_order, id
LIMIT 1
`, companyID, code).Scan(&item.ID, &item.Code, &item.Name, &item.Unit, &item.Cost)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserPosition{}, false, nil
		}
		return domain.UserPosition{}, false, err
	}
	return item, true, nil
}
