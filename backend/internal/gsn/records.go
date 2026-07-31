package gsn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type RecordResource struct {
	Code           string `json:"code"`
	Number         string `json:"number,omitempty"` // digit key from resource_codifier / source popravka
	OriginalCode   string `json:"originalCode,omitempty"`
	Name           string `json:"name"`
	Unit           string `json:"unit"`
	QuantityText   string `json:"quantityText"`
	UnitPriceText  string `json:"unitPriceText,omitempty"`
	UnitPriceIndex string `json:"unitPriceIndex,omitempty"`
	Determinant    string `json:"determinant,omitempty"`
	Mass           string `json:"mass,omitempty"`
}

// ResourceNumberReplacement replaces a norm resource matched by digit number.
type ResourceNumberReplacement struct {
	FromNumber string
	ToNumber   string
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
	Determinant    string           `json:"determinant,omitempty"`
	Mass           string           `json:"mass,omitempty"`
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
	for _, candidate := range recordLookupCandidates(code) {
		var detail RecordDetail
		var recordKind, costIndicators string
		err := s.db.QueryRowContext(ctx, `
		SELECT code, original_code, name, unit, COALESCE(determinant, ''), COALESCE(mass, ''), record_kind, cost_indicators
		FROM gsn.records
		WHERE code = $1
	`, candidate).Scan(&detail.Code, &detail.OriginalCode, &detail.Name, &detail.Unit, &detail.Determinant, &detail.Mass, &recordKind, &costIndicators)
		if err == nil {
			return detail, recordKind, costIndicators, nil
		}
		if err != sql.ErrNoRows {
			return RecordDetail{}, "", "", fmt.Errorf("query gsn record: %w", err)
		}
	}

	for _, candidate := range recordLookupCandidates(code) {
		var detail RecordDetail
		var recordKind, costIndicators string
		err := s.db.QueryRowContext(ctx, `
		SELECT code, original_code, name, unit, COALESCE(determinant, ''), COALESCE(mass, ''), record_kind, cost_indicators
		FROM gsn.records
		WHERE original_code = $1
	`, candidate).Scan(&detail.Code, &detail.OriginalCode, &detail.Name, &detail.Unit, &detail.Determinant, &detail.Mass, &recordKind, &costIndicators)
		if err == nil {
			return detail, recordKind, costIndicators, nil
		}
		if err != sql.ErrNoRows {
			return RecordDetail{}, "", "", fmt.Errorf("query gsn record by original code: %w", err)
		}
	}

	return RecordDetail{}, "", "", fmt.Errorf("record not found")
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

	cacheKey := recordDetailCacheKey(code, fgisSetID, district)
	if s.recordCache != nil {
		if detail, ok := s.recordCache.get(ctx, cacheKey); ok {
			return detail, nil
		}
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
		if s.recordCache != nil {
			s.recordCache.set(ctx, cacheKey, detail)
		}
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
	if s.recordCache != nil {
		s.recordCache.set(ctx, cacheKey, detail)
	}
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

type rawRecordResource struct {
	resourceCode string
	quantityText string
	number       string
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

	rawItems := make([]rawRecordResource, 0)
	numbers := make([]string, 0)
	for rows.Next() {
		var resourceCode, quantityText string
		if err := rows.Scan(&resourceCode, &quantityText); err != nil {
			return nil, fmt.Errorf("scan record resource: %w", err)
		}
		number := resourceNumberKey(resourceCode)
		rawItems = append(rawItems, rawRecordResource{
			resourceCode: resourceCode,
			quantityText: quantityText,
			number:       number,
		})
		if number != "" {
			numbers = append(numbers, number)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read record resources: %w", err)
	}
	if len(rawItems) == 0 {
		return nil, nil
	}

	codifiers, err := s.lookupResourceCodifiers(ctx, numbers)
	if err != nil {
		return nil, fmt.Errorf("lookup resource codifiers: %w", err)
	}

	codifierCodes := make([]string, 0, len(codifiers))
	for _, codifier := range codifiers {
		if codifier.Code != "" {
			codifierCodes = append(codifierCodes, codifier.Code)
		}
	}

	normInfos, err := s.lookupRecordNormInfos(ctx, codifierCodes)
	if err != nil {
		return nil, fmt.Errorf("lookup norm infos: %w", err)
	}

	var fgisRows map[string]fgisSetRow
	if strings.TrimSpace(fgisSetID) != "" {
		fgisRows, err = s.lookupFGISSetRows(ctx, fgisSetID, codifierCodes)
		if err != nil {
			return nil, fmt.Errorf("lookup fgis set rows: %w", err)
		}
	}

	return s.buildRecordResources(rawItems, codifiers, normInfos, fgisRows, district), nil
}

// ApplyResourceNumberReplacements replaces resources matched by digit number.
// QuantityText of the original resource is preserved; the target is re-resolved from GSN.
func (s *Service) ApplyResourceNumberReplacements(
	ctx context.Context,
	resources []RecordResource,
	replacements []ResourceNumberReplacement,
	fgisSetID, district string,
) ([]RecordResource, error) {
	if len(resources) == 0 || len(replacements) == 0 {
		return resources, nil
	}

	replaceTo := make(map[string]string, len(replacements))
	toNumbers := make([]string, 0, len(replacements))
	for _, rep := range replacements {
		from := strings.TrimSpace(rep.FromNumber)
		to := strings.TrimSpace(rep.ToNumber)
		if from == "" || to == "" || from == to {
			continue
		}
		replaceTo[from] = to
		toNumbers = append(toNumbers, to)
	}
	if len(replaceTo) == 0 {
		return resources, nil
	}

	targets, err := s.resolveResourcesByNumbers(ctx, toNumbers, fgisSetID, district)
	if err != nil {
		return nil, err
	}

	out := make([]RecordResource, len(resources))
	copy(out, resources)
	for i := range out {
		number := strings.TrimSpace(out[i].Number)
		if number == "" {
			number = resourceNumberKey(out[i].Code)
		}
		to, ok := replaceTo[number]
		if !ok {
			continue
		}
		qty := out[i].QuantityText
		if target, found := targets[to]; found {
			out[i] = target
			out[i].QuantityText = qty
			continue
		}
		out[i] = RecordResource{
			Code:         to,
			Number:       to,
			QuantityText: qty,
		}
	}
	return out, nil
}

func (s *Service) resolveResourcesByNumbers(
	ctx context.Context,
	numbers []string,
	fgisSetID, district string,
) (map[string]RecordResource, error) {
	unique := uniqueNonEmptyStrings(numbers)
	if len(unique) == 0 {
		return map[string]RecordResource{}, nil
	}
	if !s.Configured() {
		stubs := make(map[string]RecordResource, len(unique))
		for _, number := range unique {
			stubs[number] = RecordResource{Code: number, Number: number}
		}
		return stubs, nil
	}

	rawItems := make([]rawRecordResource, 0, len(unique))
	for _, number := range unique {
		rawItems = append(rawItems, rawRecordResource{
			resourceCode: number,
			quantityText: "",
			number:       number,
		})
	}

	codifiers, err := s.lookupResourceCodifiers(ctx, unique)
	if err != nil {
		return nil, fmt.Errorf("lookup resource codifiers: %w", err)
	}

	codifierCodes := make([]string, 0, len(codifiers))
	for _, codifier := range codifiers {
		if codifier.Code != "" {
			codifierCodes = append(codifierCodes, codifier.Code)
		}
	}

	normInfos, err := s.lookupRecordNormInfos(ctx, codifierCodes)
	if err != nil {
		return nil, fmt.Errorf("lookup norm infos: %w", err)
	}

	var fgisRows map[string]fgisSetRow
	if strings.TrimSpace(fgisSetID) != "" {
		fgisRows, err = s.lookupFGISSetRows(ctx, fgisSetID, codifierCodes)
		if err != nil {
			return nil, fmt.Errorf("lookup fgis set rows: %w", err)
		}
	}

	items := s.buildRecordResources(rawItems, codifiers, normInfos, fgisRows, district)
	byNumber := make(map[string]RecordResource, len(items))
	for _, item := range items {
		number := strings.TrimSpace(item.Number)
		if number == "" {
			continue
		}
		byNumber[number] = item
	}
	return byNumber, nil
}

func (s *Service) buildRecordResources(
	rawItems []rawRecordResource,
	codifiers map[string]ResourceCodifierRow,
	normInfos map[string]recordNormInfo,
	fgisRows map[string]fgisSetRow,
	district string,
) []RecordResource {
	items := make([]RecordResource, 0, len(rawItems))
	for _, raw := range rawItems {
		item := RecordResource{
			Code:         raw.number,
			Number:       raw.number,
			QuantityText: raw.quantityText,
		}
		if raw.number == "" {
			items = append(items, item)
			continue
		}

		codifier, found := codifiers[raw.number]
		if !found {
			items = append(items, item)
			continue
		}

		item.Name = codifier.Name
		item.Unit = codifier.Unit
		if codifier.Code == "" {
			items = append(items, item)
			continue
		}

		item.Code = codifier.Code
		normInfo, ok := normInfos[codifier.Code]
		if !ok {
			items = append(items, item)
			continue
		}

		item.OriginalCode = normInfo.OriginalCode
		item.Determinant = normInfo.Determinant
		item.Mass = normInfo.Mass
		if fgisRow, fgisFound := fgisRows[codifier.Code]; fgisFound {
			item.UnitPriceText, item.UnitPriceIndex = resolveResourceUnitPrice(
				fgisRow.Prices, fgisRow.Indexes, normInfo.CostIndicators, district,
			)
		}
		items = append(items, item)
	}
	return items
}
