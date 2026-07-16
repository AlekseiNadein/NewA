package gsn

import (
	"context"
	"fmt"
	"strings"
)

type SearchMatch struct {
	Node Node   `json:"node"`
	Path []Node `json:"path"`
}

func (s *Service) SearchHierarchy(ctx context.Context, supplementCode, query string, limit int) ([]SearchMatch, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}
	supplementCode = strings.TrimSpace(supplementCode)
	query = strings.TrimSpace(query)
	if supplementCode == "" {
		return nil, fmt.Errorf("supplement code is required")
	}
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT code
		FROM (
			SELECT DISTINCT ON (h.code) h.code, h.line_no
			FROM gsn.hierarchy h
			LEFT JOIN gsn.hierarchy_record_refs refs
				ON refs.supplement_code = h.supplement_code
				AND refs.hierarchy_code = h.code
			LEFT JOIN gsn.records rec ON rec.code = refs.record_code
			WHERE h.supplement_code = $1
				AND (
					h.name ILIKE $2
					OR h.code ILIKE $2
					OR COALESCE(rec.original_code, '') ILIKE $2
					OR COALESCE(rec.code, '') ILIKE $2
					OR COALESCE(rec.name, '') ILIKE $2
				)
			ORDER BY h.code, h.line_no
		) matches
		ORDER BY line_no
		LIMIT $3
	`, supplementCode, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("query gsn hierarchy search: %w", err)
	}
	defer rows.Close()

	matchCodes := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan gsn hierarchy search code: %w", err)
		}
		matchCodes = append(matchCodes, code)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn hierarchy search codes: %w", err)
	}
	if len(matchCodes) == 0 {
		return []SearchMatch{}, nil
	}

	pathPlaceholders, pathArgs := sqlInClause(2, matchCodes)
	pathArgs = append([]any{supplementCode}, pathArgs...)
	pathRows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		WITH RECURSIVE paths AS (
			SELECT code AS match_code, code, parent_code, 0 AS depth
			FROM gsn.hierarchy
			WHERE supplement_code = $1 AND code IN (%s)
			UNION ALL
			SELECT p.match_code, h.code, h.parent_code, p.depth + 1
			FROM paths p
			JOIN gsn.hierarchy h
				ON h.supplement_code = $1
				AND h.code = p.parent_code
			WHERE p.parent_code IS NOT NULL
		)
		SELECT match_code, code
		FROM paths
		WHERE depth > 0
		ORDER BY match_code, depth DESC
	`, pathPlaceholders), pathArgs...)
	if err != nil {
		return nil, fmt.Errorf("query gsn hierarchy search paths: %w", err)
	}
	defer pathRows.Close()

	pathCodesByMatch := make(map[string][]string, len(matchCodes))
	allCodes := make(map[string]struct{}, len(matchCodes)*4)
	for _, code := range matchCodes {
		allCodes[code] = struct{}{}
		pathCodesByMatch[code] = []string{}
	}
	for pathRows.Next() {
		var matchCode string
		var ancestorCode string
		if err := pathRows.Scan(&matchCode, &ancestorCode); err != nil {
			return nil, fmt.Errorf("scan gsn hierarchy search path: %w", err)
		}
		pathCodesByMatch[matchCode] = append(pathCodesByMatch[matchCode], ancestorCode)
		allCodes[ancestorCode] = struct{}{}
	}
	if err := pathRows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn hierarchy search paths: %w", err)
	}

	codeList := make([]string, 0, len(allCodes))
	for code := range allCodes {
		codeList = append(codeList, code)
	}

	nodesByCode, err := s.listHierarchyNodesByCodes(ctx, supplementCode, codeList)
	if err != nil {
		return nil, err
	}

	matches := make([]SearchMatch, 0, len(matchCodes))
	for _, matchCode := range matchCodes {
		node, ok := nodesByCode[matchCode]
		if !ok {
			continue
		}
		path := make([]Node, 0, len(pathCodesByMatch[matchCode]))
		for _, ancestorCode := range pathCodesByMatch[matchCode] {
			if ancestor, found := nodesByCode[ancestorCode]; found {
				path = append(path, ancestor)
			}
		}
		matches = append(matches, SearchMatch{
			Node: node,
			Path: path,
		})
	}

	return matches, nil
}

func (s *Service) listHierarchyNodesByCodes(ctx context.Context, supplementCode string, codes []string) (map[string]Node, error) {
	if len(codes) == 0 {
		return map[string]Node{}, nil
	}

	codePlaceholders, codeArgs := sqlInClause(2, codes)
	codeArgs = append([]any{supplementCode}, codeArgs...)
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
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
			AND h.code IN (%s)
	`, codePlaceholders), codeArgs...)
	if err != nil {
		return nil, fmt.Errorf("query gsn hierarchy nodes by codes: %w", err)
	}
	defer rows.Close()

	nodesByCode := make(map[string]Node, len(codes))
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
			return nil, fmt.Errorf("scan gsn hierarchy node: %w", err)
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
		nodesByCode[node.Code] = node
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn hierarchy nodes by codes: %w", err)
	}

	return nodesByCode, nil
}

func sqlInClause(startArg int, values []string) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for i, value := range values {
		placeholders[i] = fmt.Sprintf("$%d", startArg+i)
		args[i] = value
	}
	return strings.Join(placeholders, ", "), args
}

func uniqueNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
