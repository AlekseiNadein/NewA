package gsn

import (
	"context"
	"database/sql"
	"errors"
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
	OriginalCode     string
	CostIndicators   string
}

func (s *Service) lookupRecordNormInfo(ctx context.Context, code string) (recordNormInfo, bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return recordNormInfo{}, false, nil
	}

	var info recordNormInfo
	err := s.db.QueryRowContext(ctx, `
		SELECT original_code, cost_indicators
		FROM gsn.records
		WHERE code = $1
	`, code).Scan(&info.OriginalCode, &info.CostIndicators)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return recordNormInfo{}, false, nil
		}
		return recordNormInfo{}, false, fmt.Errorf("query gsn record norm info: %w", err)
	}
	return info, true, nil
}

func (s *Service) lookupFGISSetRow(ctx context.Context, setID, code string) (prices, indexes string, found bool, err error) {
	setID = strings.TrimSpace(setID)
	code = strings.TrimSpace(code)
	if setID == "" || code == "" {
		return "", "", false, nil
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT prices, indexes
		FROM fgis_cs.set_rows
		WHERE set_id = $1 AND code = $2
	`, setID, code).Scan(&prices, &indexes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", false, nil
		}
		return "", "", false, fmt.Errorf("query fgis_cs set row: %w", err)
	}
	return prices, indexes, true, nil
}
