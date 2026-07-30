package spec_test

import (
	"encoding/json"
	"testing"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"gopkg.in/yaml.v3"
)

func TestNumberNormalizesYAMLNumbers(t *testing.T) {
	tests := map[string]string{
		"1.00": "1",
		"1e2":  "100",
		"+0":   "0",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := parseNumber(input)
			if err != nil {
				t.Fatalf("parse Number: %v", err)
			}
			if got.String() != want {
				t.Fatalf("Number.String() = %q, want %q", got.String(), want)
			}
		})
	}
}

func TestNumberRejectsNonNumericYAMLScalars(t *testing.T) {
	for _, input := range []string{`"1.00"`, "value", "true", ".inf"} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseNumber(input); err == nil {
				t.Fatal("parse Number error = nil, want validation error")
			}
		})
	}
}

func TestNumberPreservesExactJSONNumber(t *testing.T) {
	number, err := parseNumber("9007199254740993")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		Number spec.Number `json:"number"`
	}{Number: number})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"number":9007199254740993}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
	if got := number.Decimal().String(); got != "9007199254740993" {
		t.Fatalf("Decimal() = %q", got)
	}
}

func parseNumber(input string) (spec.Number, error) {
	var document struct {
		Number spec.Number `yaml:"number"`
	}
	err := yaml.Unmarshal([]byte("number: "+input+"\n"), &document)
	return document.Number, err
}
