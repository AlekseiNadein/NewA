package store

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// ParseSourceDataQuantity parses the second field of a source-data position line.
// The value may be an arithmetic expression (often in parentheses) with an optional
// trailing [N] suffix that rounds the result to N decimal places.
func ParseSourceDataQuantity(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	expr := raw
	decimals := -1
	if open := strings.LastIndex(expr, "["); open >= 0 && strings.HasSuffix(expr, "]") {
		inside := strings.TrimSpace(expr[open+1 : len(expr)-1])
		if inside == "" {
			return 0, fmt.Errorf("empty quantity precision in %q", raw)
		}
		n, err := strconv.Atoi(inside)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid quantity precision %q in %q", inside, raw)
		}
		decimals = n
		expr = strings.TrimSpace(expr[:open])
	}

	var (
		value float64
		err   error
	)
	if isSourceDataQuantityExpression(expr) {
		value, err = evaluateArithmeticExpression(expr)
	} else {
		value, err = parseLocalizedNumber(expr)
	}
	if err != nil {
		return 0, err
	}
	if !mathIsFinite(value) {
		return 0, fmt.Errorf("quantity is not finite in %q", raw)
	}
	if decimals >= 0 {
		value = roundToDecimals(value, decimals)
	}
	return value, nil
}

func roundToDecimals(value float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Round(value*factor) / factor
}

func isSourceDataQuantityExpression(expr string) bool {
	return strings.Contains(expr, "(") || strings.ContainsAny(expr, "+-*/.:")
}

func parseLocalizedNumber(raw string) (float64, error) {
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

func evaluateArithmeticExpression(expr string) (float64, error) {
	tokens, err := tokenizeArithmeticExpression(expr)
	if err != nil {
		return 0, err
	}
	parser := &arithParser{tokens: tokens}
	value, err := parser.parseExpression()
	if err != nil {
		return 0, err
	}
	if parser.pos != len(parser.tokens) {
		return 0, fmt.Errorf("unexpected trailing input in %q", expr)
	}
	return value, nil
}

type arithTokenKind int

const (
	tokenNumber arithTokenKind = iota
	tokenPlus
	tokenMinus
	tokenMul
	tokenDiv
	tokenLParen
	tokenRParen
)

type arithToken struct {
	kind  arithTokenKind
	value float64
}

type arithParser struct {
	tokens []arithToken
	pos    int
}

func (p *arithParser) parseExpression() (float64, error) {
	value, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.tokens) {
		switch p.tokens[p.pos].kind {
		case tokenPlus:
			p.pos++
			rhs, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			value += rhs
		case tokenMinus:
			p.pos++
			rhs, err := p.parseTerm()
			if err != nil {
				return 0, err
			}
			value -= rhs
		default:
			return value, nil
		}
	}
	return value, nil
}

func (p *arithParser) parseTerm() (float64, error) {
	value, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for p.pos < len(p.tokens) {
		switch p.tokens[p.pos].kind {
		case tokenMul:
			p.pos++
			rhs, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			value *= rhs
		case tokenDiv:
			p.pos++
			rhs, err := p.parseFactor()
			if err != nil {
				return 0, err
			}
			if rhs == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			value /= rhs
		default:
			return value, nil
		}
	}
	return value, nil
}

func (p *arithParser) parseFactor() (float64, error) {
	if p.pos >= len(p.tokens) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	switch p.tokens[p.pos].kind {
	case tokenPlus:
		p.pos++
		return p.parseFactor()
	case tokenMinus:
		p.pos++
		value, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -value, nil
	case tokenLParen:
		p.pos++
		value, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if p.pos >= len(p.tokens) || p.tokens[p.pos].kind != tokenRParen {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return value, nil
	case tokenNumber:
		value := p.tokens[p.pos].value
		p.pos++
		return value, nil
	default:
		return 0, fmt.Errorf("unexpected token in expression")
	}
}

func tokenizeArithmeticExpression(expr string) ([]arithToken, error) {
	expr = strings.ReplaceAll(strings.TrimSpace(expr), " ", "")
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}

	tokens := make([]arithToken, 0)
	for i := 0; i < len(expr); {
		ch := expr[i]
		switch ch {
		case '+':
			tokens = append(tokens, arithToken{kind: tokenPlus})
			i++
		case '-':
			tokens = append(tokens, arithToken{kind: tokenMinus})
			i++
		case '*', '.':
			tokens = append(tokens, arithToken{kind: tokenMul})
			i++
		case '/', ':':
			tokens = append(tokens, arithToken{kind: tokenDiv})
			i++
		case '(':
			tokens = append(tokens, arithToken{kind: tokenLParen})
			i++
		case ')':
			tokens = append(tokens, arithToken{kind: tokenRParen})
			i++
		default:
			if unicode.IsDigit(rune(ch)) || ch == ',' {
				j := i
				for j < len(expr) {
					c := expr[j]
					if unicode.IsDigit(rune(c)) || c == ',' {
						j++
						continue
					}
					break
				}
				value, err := parseLocalizedNumber(expr[i:j])
				if err != nil {
					return nil, err
				}
				tokens = append(tokens, arithToken{kind: tokenNumber, value: value})
				i = j
				continue
			}
			return nil, fmt.Errorf("invalid character %q in expression %q", ch, expr)
		}
	}
	return tokens, nil
}

func mathIsFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
