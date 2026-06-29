package gsn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type RecordResource struct {
	Code          string `json:"code"`
	OriginalCode  string `json:"originalCode,omitempty"`
	Name          string `json:"name"`
	Unit          string `json:"unit"`
	QuantityText  string `json:"quantityText"`
	UnitPriceText  string `json:"unitPriceText,omitempty"`
	UnitPriceIndex string `json:"unitPriceIndex,omitempty"`
}

type RecordDetail struct {
	Code           string           `json:"code"`
	OriginalCode   string           `json:"originalCode"`
	Name           string           `json:"name"`
	Unit           string           `json:"unit"`
	IsWork         bool             `json:"isWork"`
	HasResources   bool             `json:"hasResources"`
	UnitPriceText  string           `json:"unitPriceText,omitempty"`
	UnitPriceIndex string           `json:"unitPriceIndex,omitempty"`
	Resources      []RecordResource `json:"resources,omitempty"`
}

func ExtractPositionCipher(firstField string) string {
	raw := strings.TrimSpace(firstField)
	if raw == "" {
		return ""
	}
	cutAt := len(raw)
	for _, sep := range []string{"(", " ", "#"} {
		if i := strings.Index(raw, sep); i >= 0 && i < cutAt {
			cutAt = i
		}
	}
	return strings.TrimSpace(raw[:cutAt])
}

func parseNormListRefs(normList string) []string {
	normList = strings.TrimSpace(normList)
	if normList == "" {
		return nil
	}

	refs := make([]string, 0)
	for _, part := range strings.Split(normList, "/") {
		part = strings.Trim(strings.TrimSpace(part), ";")
		if part != "" {
			refs = append(refs, part)
		}
	}
	return refs
}

func (s *Service) lookupRecordDetail(ctx context.Context, code string) (RecordDetail, string, string, error) {
	var detail RecordDetail
	var recordKind, costIndicators string
	err := s.db.QueryRowContext(ctx, `
		SELECT code, original_code, name, unit, record_kind, cost_indicators
		FROM gsn.records
		WHERE code = $1
	`, code).Scan(&detail.Code, &detail.OriginalCode, &detail.Name, &detail.Unit, &recordKind, &costIndicators)
	if err == nil {
		return detail, recordKind, costIndicators, nil
	}
	if err != sql.ErrNoRows {
		return RecordDetail{}, "", "", fmt.Errorf("query gsn record: %w", err)
	}

	err = s.db.QueryRowContext(ctx, `
		SELECT code, original_code, name, unit, record_kind, cost_indicators
		FROM gsn.records
		WHERE original_code = $1
	`, code).Scan(&detail.Code, &detail.OriginalCode, &detail.Name, &detail.Unit, &recordKind, &costIndicators)
	if err != nil {
		if err == sql.ErrNoRows {
			return RecordDetail{}, "", "", fmt.Errorf("record not found")
		}
		return RecordDetail{}, "", "", fmt.Errorf("query gsn record by original code: %w", err)
	}
	return detail, recordKind, costIndicators, nil
}

func (s *Service) applyRecordSelfPricing(ctx context.Context, code, fgisSetID, district, costIndicators string) (string, string, error) {
	if strings.TrimSpace(fgisSetID) == "" {
		return "", "", nil
	}
	prices, indexes, found, err := s.lookupFGISSetRow(ctx, fgisSetID, code)
	if err != nil {
		return "", "", err
	}
	if !found {
		return "", "", nil
	}
	unitPriceText, unitPriceIndex := resolveResourceUnitPrice(prices, indexes, costIndicators, district)
	return unitPriceText, unitPriceIndex, nil
}

func normalizeRecordDetailOriginalCode(detail *RecordDetail) {
	if detail == nil {
		return
	}
	detail.OriginalCode = strings.TrimSpace(detail.OriginalCode)
	if detail.OriginalCode == "" {
		detail.OriginalCode = strings.TrimSpace(detail.Code)
	}
}

func (s *Service) GetRecordDetail(ctx context.Context, code, fgisSetID, district string) (RecordDetail, error) {
	if !s.Configured() {
		return RecordDetail{}, ErrNotConfigured
	}

	code = ExtractPositionCipher(code)
	if code == "" {
		return RecordDetail{}, fmt.Errorf("record code is required")
	}

	detail, recordKind, costIndicators, err := s.lookupRecordDetail(ctx, code)
	if err != nil {
		return RecordDetail{}, err
	}

	detail.IsWork = isWorkNormRecord(detail.Code, recordKind)
	if detail.IsWork {
		resources, err := s.listRecordResources(ctx, detail.Code, fgisSetID, district)
		if err != nil {
			return RecordDetail{}, err
		}
		detail.Resources = resources
		detail.HasResources = len(resources) > 0
		normalizeRecordDetailOriginalCode(&detail)
		return detail, nil
	}

	detail.HasResources = false
	unitPriceText, unitPriceIndex, err := s.applyRecordSelfPricing(ctx, detail.Code, fgisSetID, district, costIndicators)
	if err != nil {
		return RecordDetail{}, err
	}
	detail.UnitPriceText = unitPriceText
	detail.UnitPriceIndex = unitPriceIndex
	normalizeRecordDetailOriginalCode(&detail)
	return detail, nil
}

func (s *Service) ListHierarchyRecords(ctx context.Context, supplementCode, hierarchyCode, fgisSetID, district string) ([]RecordDetail, error) {
	if !s.Configured() {
		return nil, ErrNotConfigured
	}

	supplementCode = strings.TrimSpace(supplementCode)
	hierarchyCode = strings.TrimSpace(hierarchyCode)
	if supplementCode == "" || hierarchyCode == "" {
		return nil, fmt.Errorf("supplement and hierarchy codes are required")
	}

	codes := make([]string, 0)
	rows, err := s.db.QueryContext(ctx, `
		SELECT record_code
		FROM gsn.hierarchy_record_refs
		WHERE supplement_code = $1 AND hierarchy_code = $2
		ORDER BY ordinal
	`, supplementCode, hierarchyCode)
	if err != nil {
		return nil, fmt.Errorf("query hierarchy record refs: %w", err)
	}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan hierarchy record ref: %w", err)
		}
		codes = append(codes, code)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read hierarchy record refs: %w", err)
	}

	if len(codes) == 0 {
		var normList string
		err := s.db.QueryRowContext(ctx, `
			SELECT raw_norm_list
			FROM gsn.hierarchy
			WHERE supplement_code = $1 AND code = $2
		`, supplementCode, hierarchyCode).Scan(&normList)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("query hierarchy norm list: %w", err)
		}
		codes = parseNormListRefs(normList)
	}

	items := make([]RecordDetail, 0, len(codes))
	for _, code := range codes {
		detail, err := s.GetRecordDetail(ctx, code, fgisSetID, district)
		if err != nil {
			continue
		}
		items = append(items, detail)
	}
	return items, nil
}

func (s *Service) listRecordResources(ctx context.Context, recordCode, fgisSetID, district string) ([]RecordResource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT resource_code, quantity_text
		FROM gsn.record_resources
		WHERE record_code = $1
		ORDER BY ordinal
	`, recordCode)
	if err != nil {
		return nil, fmt.Errorf("query record resources: %w", err)
	}
	defer rows.Close()

	items := make([]RecordResource, 0)
	for rows.Next() {
		var resourceCode, quantityText string
		if err := rows.Scan(&resourceCode, &quantityText); err != nil {
			return nil, fmt.Errorf("scan record resource: %w", err)
		}

		number := resourceNumberKey(resourceCode)
		item := RecordResource{
			Code:         number,
			QuantityText: quantityText,
		}

		if number != "" {
			codifier, found, err := s.lookupResourceCodifier(ctx, number)
			if err != nil {
				return nil, fmt.Errorf("lookup resource codifier %q: %w", number, err)
			}
			if found {
				item.Name = codifier.Name
				item.Unit = codifier.Unit
				if codifier.Code != "" {
					item.Code = codifier.Code
					normInfo, ok, err := s.lookupRecordNormInfo(ctx, codifier.Code)
					if err != nil {
						return nil, fmt.Errorf("lookup norm info %q: %w", codifier.Code, err)
					}
					if ok {
						item.OriginalCode = normInfo.OriginalCode
						if fgisSetID != "" {
							prices, indexes, fgisFound, err := s.lookupFGISSetRow(ctx, fgisSetID, codifier.Code)
							if err != nil {
								return nil, fmt.Errorf("lookup fgis set row %q: %w", codifier.Code, err)
							}
							if fgisFound {
								item.UnitPriceText, item.UnitPriceIndex = resolveResourceUnitPrice(
									prices, indexes, normInfo.CostIndicators, district,
								)
							}
						}
					}
				}
			}
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read record resources: %w", err)
	}
	return items, nil
}
