package spec

import (
	"errors"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

// Number is an exact, canonical decimal representation of a YAML number.
type Number struct {
	canonical string
}

// UnmarshalYAML accepts numeric YAML scalars without converting through float64.
func (n *Number) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" && node.Tag != "!!float" {
		return errors.New("must be a YAML number")
	}
	value, err := decimal.NewFromString(node.Value)
	if err != nil {
		return errors.New("must be a finite YAML number")
	}
	n.canonical = value.String()
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
