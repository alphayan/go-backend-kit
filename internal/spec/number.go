package spec

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

const maxCanonicalNumberLength = 1 << 20

// Number is an exact, canonical decimal representation of a YAML number.
type Number struct {
	canonical string
}

// UnmarshalYAML accepts numeric YAML scalars without converting through float64.
func (n *Number) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" && node.Tag != "!!float" {
		return errors.New("must be a YAML number")
	}
	canonical, err := canonicalNumberString(node.Value)
	if err != nil {
		return fmt.Errorf("must be a bounded finite YAML number: %w", err)
	}
	n.canonical = canonical
	return nil
}

// String returns the canonical base-10 representation.
func (n Number) String() string {
	return n.canonical
}

// Decimal returns the exact decimal value.
func (n Number) Decimal() decimal.Decimal {
	return decimal.RequireFromString(n.canonical)
}

// MarshalJSON preserves Number as an unquoted JSON number.
func (n Number) MarshalJSON() ([]byte, error) {
	if n.canonical == "" {
		return nil, errors.New("cannot encode an empty number")
	}
	return []byte(n.canonical), nil
}

func canonicalNumberString(raw string) (string, error) {
	if len(raw) > maxCanonicalNumberLength {
		return "", errors.New("number is too long")
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return "", errors.New("number is invalid")
	}
	return canonicalNumber(value)
}

func canonicalNumber(value decimal.Decimal) (string, error) {
	if value.IsZero() {
		return "0", nil
	}
	digits := int64(value.NumDigits())
	exponent := int64(value.Exponent())
	var length int64
	switch {
	case exponent >= 0:
		length = digits + exponent
	case digits > -exponent:
		length = digits + 1
	default:
		length = 2 - exponent
	}
	if value.IsNegative() {
		length++
	}
	if length > maxCanonicalNumberLength {
		return "", errors.New("canonical number is too long")
	}
	return value.String(), nil
}
