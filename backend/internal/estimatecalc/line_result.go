package estimatecalc

import (
	"fmt"
	"strconv"
	"strings"

	"nav-saas-mvp/backend/internal/gsn"
)

// ResourceContribution is one resource's contribution to a calculated estimate position.
type ResourceContribution struct {
	Code          string
	Determinant   string
	Consumption   float64 // total consumption for the position volume
	Name          string
	Unit          string
	EstimatePrice float64 // unit estimate price; 0 when unknown
	SellingPrice  *float64
	TransportCost *float64
	Mass          string
	CargoClass    string
	Corrections   string
}

// LineCalcSnapshot is the persisted calculation snapshot for one position.
type LineCalcSnapshot struct {
	Code          string
	OriginalCode  string
	Name          string
	Unit          string
	Determinant   string
	Quantity      float64
	UnitPrice     float64
	Total         float64
	ResourcesText string
	Resources     []ResourceContribution
}

// BuildLineCalcSnapshot prices a GSN record and builds resource contributions for the volume.
func BuildLineCalcSnapshot(record gsn.RecordDetail, quantity float64) (LineCalcSnapshot, error) {
	pricing, err := LinePricingFromRecord(record, quantity)
	if err != nil {
		return LineCalcSnapshot{}, err
	}

	snap := LineCalcSnapshot{
		Code:         strings.TrimSpace(record.Code),
		OriginalCode: strings.TrimSpace(record.OriginalCode),
		Name:         strings.TrimSpace(record.Name),
		Unit:         strings.TrimSpace(record.Unit),
		Determinant:  strings.TrimSpace(record.Determinant),
		Quantity:     pricing.Quantity,
		UnitPrice:    pricing.UnitPrice,
		Total:        pricing.Total,
	}
	if snap.OriginalCode == "" {
		snap.OriginalCode = snap.Code
	}

	if !record.IsWork {
		unitPrice, err := ResourceUnitPrice(record.UnitPriceText, record.UnitPriceIndex)
		if err != nil {
			return LineCalcSnapshot{}, err
		}
		unitPrice = RoundMoney(unitPrice)
		snap.Resources = []ResourceContribution{{
			Code:          snap.Code,
			Determinant:   strings.TrimSpace(record.Determinant),
			Consumption:   quantity,
			Name:          snap.Name,
			Unit:          snap.Unit,
			EstimatePrice: unitPrice,
			Mass:          strings.TrimSpace(record.Mass),
		}}
		snap.ResourcesText = FormatResourcesText(snap.Resources)
		return snap, nil
	}

	merged := make(map[string]ResourceContribution)
	order := make([]string, 0)
	for _, resource := range record.Resources {
		unitPrice, err := ResourceUnitPrice(resource.UnitPriceText, resource.UnitPriceIndex)
		if err != nil {
			return LineCalcSnapshot{}, fmt.Errorf("resource %q: %w", resource.Code, err)
		}
		consumptionPerUnit, err := ParseResourceQuantity(resource.QuantityText)
		if err != nil {
			return LineCalcSnapshot{}, fmt.Errorf("resource %q: %w", resource.Code, err)
		}
		code := strings.TrimSpace(resource.Code)
		if code == "" {
			continue
		}
		determinant := strings.TrimSpace(resource.Determinant)
		key := resourceAggKey(code, determinant)
		totalConsumption := consumptionPerUnit * quantity
		if existing, ok := merged[key]; ok {
			existing.Consumption += totalConsumption
			merged[key] = existing
			continue
		}
		merged[key] = ResourceContribution{
			Code:          code,
			Determinant:   determinant,
			Consumption:   totalConsumption,
			Name:          strings.TrimSpace(resource.Name),
			Unit:          strings.TrimSpace(resource.Unit),
			EstimatePrice: RoundMoney(unitPrice),
			Mass:          strings.TrimSpace(resource.Mass),
		}
		order = append(order, key)
	}

	snap.Resources = make([]ResourceContribution, 0, len(order))
	for _, key := range order {
		snap.Resources = append(snap.Resources, merged[key])
	}
	snap.ResourcesText = FormatResourcesText(snap.Resources)
	return snap, nil
}

func resourceAggKey(code, determinant string) string {
	return code + "\x00" + determinant
}

// ApplyDeterminantAssignment sets the line determinant from a source-data (=...) correction.
// For a resource position (self as the only resource with the same code), the resource
// determinant is updated too. Nested work resources keep their own determinants.
func ApplyDeterminantAssignment(snap *LineCalcSnapshot, determinant string) {
	if snap == nil {
		return
	}
	determinant = strings.TrimSpace(determinant)
	if determinant == "" {
		return
	}
	snap.Determinant = determinant
	lineCode := strings.TrimSpace(snap.Code)
	for i := range snap.Resources {
		if strings.TrimSpace(snap.Resources[i].Code) == lineCode {
			snap.Resources[i].Determinant = determinant
		}
	}
}

// FormatResourcesText builds "код.расход/код.расход" using comma decimals.
func FormatResourcesText(resources []ResourceContribution) string {
	if len(resources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(resources))
	for _, resource := range resources {
		code := strings.TrimSpace(resource.Code)
		if code == "" {
			continue
		}
		parts = append(parts, code+"."+formatConsumption(resource.Consumption))
	}
	return strings.Join(parts, "/")
}

func formatConsumption(value float64) string {
	if value == 0 {
		return "0"
	}
	text := strconv.FormatFloat(value, 'f', 6, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	return strings.ReplaceAll(text, ".", ",")
}
