package estimatecalc

import (
	"fmt"
	"strconv"
	"strings"

	"nav-saas-mvp/backend/internal/gsn"
)

// LinePricing is the monetary result for one estimate line at a given volume.
type LinePricing struct {
	Quantity  float64
	UnitPrice float64
	Total     float64
}

func ParseLocalizedNumber(raw string) (float64, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""), ",", ".")
	if normalized == "" {
		return 0, nil
	}
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", raw)
	}
	return value, nil
}

func ResourceUnitPrice(unitPriceText, unitPriceIndex string) (float64, error) {
	text := strings.TrimSpace(unitPriceText)
	indexText := strings.TrimSpace(unitPriceIndex)
	if star := strings.Index(text, "*"); star >= 0 {
		if indexText == "" {
			indexText = strings.TrimSpace(text[star+1:])
		}
		text = strings.TrimSpace(text[:star])
	}

	base, err := ParseLocalizedNumber(text)
	if err != nil {
		return 0, err
	}
	index, err := ParseLocalizedNumber(indexText)
	if err != nil {
		return 0, err
	}
	if index != 0 {
		return base * index, nil
	}
	if base != 0 {
		return base, nil
	}
	return 0, nil
}

func ParseResourceQuantity(text string) (float64, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(text), " ", "")
	if normalized == "" {
		return 0, nil
	}
	if strings.EqualFold(normalized, "П") {
		return 0, nil
	}
	normalized = strings.ReplaceAll(normalized, ",", ".")
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid resource quantity %q", text)
	}
	return value, nil
}

func LinePricingFromRecord(record gsn.RecordDetail, quantity float64) (LinePricing, error) {
	if quantity < 0 {
		return LinePricing{}, fmt.Errorf("quantity must be non-negative")
	}

	if !record.IsWork {
		unitPrice, err := ResourceUnitPrice(record.UnitPriceText, record.UnitPriceIndex)
		if err != nil {
			return LinePricing{}, err
		}
		return LinePricing{
			Quantity:  quantity,
			UnitPrice: unitPrice,
			Total:     unitPrice * quantity,
		}, nil
	}

	if len(record.Resources) == 0 {
		return LinePricing{}, fmt.Errorf("work position %q has no resources", record.Code)
	}

	var total float64
	for _, resource := range record.Resources {
		unitPrice, err := ResourceUnitPrice(resource.UnitPriceText, resource.UnitPriceIndex)
		if err != nil {
			return LinePricing{}, fmt.Errorf("resource %q: %w", resource.Code, err)
		}
		consumption, err := ParseResourceQuantity(resource.QuantityText)
		if err != nil {
			return LinePricing{}, fmt.Errorf("resource %q: %w", resource.Code, err)
		}
		total += unitPrice * consumption * quantity
	}

	unitPrice := total
	if quantity > 0 {
		unitPrice = total / quantity
	}
	return LinePricing{
		Quantity:  quantity,
		UnitPrice: unitPrice,
		Total:     total,
	}, nil
}
