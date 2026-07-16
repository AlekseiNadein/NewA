package gsn

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func extractBasePrice(costIndicators string) string {
	costIndicators = strings.TrimSpace(costIndicators)
	if costIndicators == "" {
		return ""
	}

	if parts := strings.Split(costIndicators, "##"); len(parts) >= 2 {
		return strings.TrimSpace(parts[len(parts)-1])
	}

	hashParts := strings.Split(costIndicators, "#")
	if len(hashParts) >= 2 {
		last := strings.TrimSpace(hashParts[len(hashParts)-1])
		if colon := strings.LastIndex(last, ":"); colon >= 0 {
			return strings.TrimSpace(last[colon+1:])
		}
		return last
	}

	if colon := strings.LastIndex(costIndicators, ":"); colon >= 0 {
		return strings.TrimSpace(costIndicators[colon+1:])
	}

	return costIndicators
}

func normalizeDistrictRegionCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if n, err := strconv.Atoi(code); err == nil {
		return strconv.Itoa(n)
	}
	return code
}

func normalizeDistrictZone(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return ""
	}
	n, err := strconv.Atoi(zone)
	if err != nil || n < 1 || n > 11 {
		return ""
	}
	return strconv.Itoa(n)
}

func normalizeDistrictKey(district string) string {
	district = strings.TrimSpace(district)
	if district == "" {
		return ""
	}

	dotIndex := strings.Index(district, ".")
	if dotIndex < 0 {
		return normalizeDistrictRegionCode(district)
	}

	regionCode := normalizeDistrictRegionCode(district[:dotIndex])
	zonePart := district[dotIndex+1:]
	if strings.Contains(zonePart, ".") {
		return regionCode
	}

	zone := normalizeDistrictZone(zonePart)
	if zone == "" {
		return regionCode
	}
	return regionCode + "." + zone
}

func pickFGISValueByDistrict(values, district string) string {
	values = strings.TrimSpace(values)
	district = normalizeDistrictKey(district)
	if values == "" || district == "" {
		return ""
	}

	for _, part := range strings.Split(values, "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.Index(part, ":")
		if colon <= 0 {
			continue
		}
		key := normalizeDistrictKey(part[:colon])
		value := strings.TrimSpace(part[colon+1:])
		if key == district && value != "" {
			return value
		}
	}
	return ""
}

func resolveResourceUnitPrice(prices, indexes, costIndicators, district string) (unitPriceText, unitPriceIndex string) {
	prices = strings.TrimSpace(prices)
	indexes = strings.TrimSpace(indexes)

	if prices != "" {
		return pickFGISValueByDistrict(prices, district), ""
	}
	if indexes == "" {
		return "", ""
	}

	pickedIndex := pickFGISValueByDistrict(indexes, district)
	if pickedIndex == "" {
		return "", ""
	}

	basePrice := extractBasePrice(costIndicators)
	if basePrice == "" {
		return "", ""
	}

	return basePrice, pickedIndex
}

type recordNormInfo struct {
	OriginalCode   string
	CostIndicators string
}

type fgisSetRow struct {
	Prices  string
	Indexes string
}

func (s *Service) lookupRecordNormInfo(ctx context.Context, code string) (recordNormInfo, bool, error) {
	items, err := s.lookupRecordNormInfos(ctx, []string{code})
	if err != nil {
		return recordNormInfo{}, false, err
	}
	info, ok := items[strings.TrimSpace(code)]
	return info, ok, nil
}

func (s *Service) lookupRecordNormInfos(ctx context.Context, codes []string) (map[string]recordNormInfo, error) {
	unique := uniqueNonEmptyStrings(codes)
	if len(unique) == 0 {
		return map[string]recordNormInfo{}, nil
	}

	inClause, args := sqlInClause(1, unique)
	query := fmt.Sprintf(`
		SELECT code, original_code, cost_indicators
		FROM gsn.records
		WHERE code IN (%s)
	`, inClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query gsn record norm infos: %w", err)
	}
	defer rows.Close()

	items := make(map[string]recordNormInfo, len(unique))
	for rows.Next() {
		var code string
		var info recordNormInfo
		if err := rows.Scan(&code, &info.OriginalCode, &info.CostIndicators); err != nil {
			return nil, fmt.Errorf("scan gsn record norm info: %w", err)
		}
		items[code] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read gsn record norm infos: %w", err)
	}
	return items, nil
}

func (s *Service) lookupFGISSetRow(ctx context.Context, setID, code string) (prices, indexes string, found bool, err error) {
	rows, err := s.lookupFGISSetRows(ctx, setID, []string{code})
	if err != nil {
		return "", "", false, err
	}
	row, ok := rows[strings.TrimSpace(code)]
	if !ok {
		return "", "", false, nil
	}
	return row.Prices, row.Indexes, true, nil
}

func (s *Service) lookupFGISSetRows(ctx context.Context, setID string, codes []string) (map[string]fgisSetRow, error) {
	setID = strings.TrimSpace(setID)
	unique := uniqueNonEmptyStrings(codes)
	if setID == "" || len(unique) == 0 {
		return map[string]fgisSetRow{}, nil
	}

	inClause, codeArgs := sqlInClause(2, unique)
	args := append([]any{setID}, codeArgs...)
	query := fmt.Sprintf(`
		SELECT code, prices, indexes
		FROM fgis_cs.set_rows
		WHERE set_id = $1 AND code IN (%s)
	`, inClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query fgis_cs set rows: %w", err)
	}
	defer rows.Close()

	items := make(map[string]fgisSetRow, len(unique))
	for rows.Next() {
		var code string
		var row fgisSetRow
		if err := rows.Scan(&code, &row.Prices, &row.Indexes); err != nil {
			return nil, fmt.Errorf("scan fgis_cs set row: %w", err)
		}
		items[code] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read fgis_cs set rows: %w", err)
	}
	return items, nil
}
