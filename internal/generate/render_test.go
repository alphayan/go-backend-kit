package generate

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"github.com/shopspring/decimal"
)

func TestFloatSampleSatisfiesExactBound(t *testing.T) {
	tests := map[string]struct {
		constraint string
		valid      func(decimal.Decimal, decimal.Decimal) bool
	}{
		"minimum": {
			constraint: "min: 12.5000000000000001",
			valid:      func(value, bound decimal.Decimal) bool { return !value.LessThan(bound) },
		},
		"maximum": {
			constraint: "max: 12.4999999999999999",
			valid:      func(value, bound decimal.Decimal) bool { return !value.GreaterThan(bound) },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resource, err := spec.Parse([]byte(`schema_version: 1
name: Reading
table: readings
route: /readings
fields:
  - name: value
    type: float64
    ` + test.constraint + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			field := resource.Fields[0]
			sample, err := strconv.ParseFloat(sampleGoValue(field), 64)
			if err != nil {
				t.Fatal(err)
			}
			bound := field.Min
			if bound == nil {
				bound = field.Max
			}
			if !test.valid(decimal.NewFromFloat(sample), bound.Decimal()) {
				t.Fatalf("sample %s does not satisfy %s", sampleGoValue(field), strings.TrimSpace(test.constraint))
			}
		})
	}
}
