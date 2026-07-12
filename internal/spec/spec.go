// Package spec parses and validates go-backend-kit resource specifications.
package spec

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"
)

const SchemaVersion = 1

type FieldType string

const (
	TypeString  FieldType = "string"
	TypeText    FieldType = "text"
	TypeBool    FieldType = "bool"
	TypeInt32   FieldType = "int32"
	TypeInt64   FieldType = "int64"
	TypeFloat64 FieldType = "float64"
	TypeDecimal FieldType = "decimal"
	TypeTime    FieldType = "time"
	TypeUUID    FieldType = "uuid"
	TypeJSON    FieldType = "json"
)

var (
	modelNamePattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	dbNamePattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
	routePattern     = regexp.MustCompile(`^/[a-z0-9]+(?:-[a-z0-9]+)*$`)
	uuidPattern      = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	decimalPattern   = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
)

var validTypes = map[FieldType]struct{}{
	TypeString: {}, TypeText: {}, TypeBool: {}, TypeInt32: {}, TypeInt64: {},
	TypeFloat64: {}, TypeDecimal: {}, TypeTime: {}, TypeUUID: {}, TypeJSON: {},
}

var baseFields = map[string]struct{}{
	"id": {}, "created_at": {}, "updated_at": {},
}

var goKeywords = map[string]struct{}{
	"break": {}, "default": {}, "func": {}, "interface": {}, "select": {},
	"case": {}, "defer": {}, "go": {}, "map": {}, "struct": {}, "chan": {},
	"else": {}, "goto": {}, "package": {}, "switch": {}, "const": {}, "fallthrough": {},
	"if": {}, "range": {}, "type": {}, "continue": {}, "for": {}, "import": {},
	"return": {}, "var": {},
}

var reservedPackages = map[string]struct{}{
	"main": {}, "testdata": {}, "vendor": {},
}

var windowsDeviceNames = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {}, "com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {}, "lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

var generatedIdentifiers = map[string]struct{}{
	"Column": {}, "Columns": {}, "Filters": {}, "Handler": {}, "Register": {},
	"Repository": {}, "Service": {}, "NewHandler": {}, "NewRepository": {}, "NewService": {},
}

// Resource is the normalized representation used by every generator.
type Resource struct {
	SchemaVersion int     `json:"schema_version"`
	Name          string  `json:"name"`
	Package       string  `json:"package"`
	Table         string  `json:"table"`
	Route         string  `json:"route"`
	Fields        []Field `json:"fields"`
}

type Field struct {
	Name       string    `json:"name"`
	GoName     string    `json:"go_name"`
	Column     string    `json:"column"`
	Type       FieldType `json:"type"`
	Required   bool      `json:"required"`
	Nullable   bool      `json:"nullable"`
	HasDefault bool      `json:"has_default"`
	Default    any       `json:"default,omitempty"`
	Unique     bool      `json:"unique"`
	Index      bool      `json:"index"`
	Enum       []string  `json:"enum,omitempty"`
	Min        *float64  `json:"min,omitempty"`
	Max        *float64  `json:"max,omitempty"`
	MaxLength  *int      `json:"max_length,omitempty"`
	Searchable bool      `json:"searchable"`
	Filterable bool      `json:"filterable"`
	Sortable   bool      `json:"sortable"`
}

type rawResource struct {
	SchemaVersion int        `yaml:"schema_version"`
	Name          string     `yaml:"name"`
	Table         string     `yaml:"table"`
	Route         string     `yaml:"route"`
	Fields        []rawField `yaml:"fields"`
}

type rawField struct {
	Name       string    `yaml:"name"`
	Type       FieldType `yaml:"type"`
	Required   bool      `yaml:"required"`
	Nullable   *bool     `yaml:"nullable"`
	Default    yaml.Node `yaml:"default"`
	Unique     bool      `yaml:"unique"`
	Index      bool      `yaml:"index"`
	Enum       []string  `yaml:"enum"`
	Min        *float64  `yaml:"min"`
	Max        *float64  `yaml:"max"`
	MaxLength  *int      `yaml:"max_length"`
	Searchable bool      `yaml:"searchable"`
	Filterable bool      `yaml:"filterable"`
	Sortable   bool      `yaml:"sortable"`
}

// Parse decodes one strict Schema v1 YAML document and returns its normalized form.
func Parse(data []byte) (Resource, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var raw rawResource
	if err := decoder.Decode(&raw); err != nil {
		return Resource{}, fmt.Errorf("decode resource: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Resource{}, errors.New("decode resource: multiple YAML documents are not allowed")
		}
		return Resource{}, fmt.Errorf("decode resource: %w", err)
	}

	if raw.SchemaVersion != SchemaVersion {
		return Resource{}, fmt.Errorf("schema_version must be %d", SchemaVersion)
	}
	if !modelNamePattern.MatchString(raw.Name) {
		return Resource{}, errors.New("name must be an exported Go identifier")
	}
	packageName := strings.ToLower(raw.Name)
	if _, reserved := goKeywords[packageName]; reserved {
		return Resource{}, fmt.Errorf("name %q produces reserved package name %q", raw.Name, packageName)
	}
	if _, reserved := reservedPackages[packageName]; reserved {
		return Resource{}, fmt.Errorf("name %q produces unimportable package name %q", raw.Name, packageName)
	}
	if _, reserved := windowsDeviceNames[packageName]; reserved {
		return Resource{}, fmt.Errorf("name %q produces Windows reserved path %q", raw.Name, packageName)
	}
	if _, reserved := generatedIdentifiers[raw.Name]; reserved {
		return Resource{}, fmt.Errorf("name %q collides with a generated identifier", raw.Name)
	}
	if !dbNamePattern.MatchString(raw.Table) {
		return Resource{}, errors.New("table must be a lowercase SQL identifier")
	}
	if !routePattern.MatchString(raw.Route) || strings.Contains(raw.Route, "..") {
		return Resource{}, errors.New("route must be one safe lowercase path segment")
	}

	resource := Resource{
		SchemaVersion: raw.SchemaVersion,
		Name:          raw.Name,
		Package:       packageName,
		Table:         raw.Table,
		Route:         raw.Route,
		Fields:        make([]Field, 0, len(raw.Fields)),
	}
	seen := make(map[string]struct{}, len(raw.Fields))
	seenGoNames := map[string]struct{}{"ID": {}, "CreatedAt": {}, "UpdatedAt": {}, "TableName": {}, "AfterFind": {}}
	for i, candidate := range raw.Fields {
		field, err := normalizeField(candidate)
		if err != nil {
			return Resource{}, fmt.Errorf("fields[%d] %q: %w", i, candidate.Name, err)
		}
		if _, base := baseFields[field.Name]; base {
			return Resource{}, fmt.Errorf("fields[%d] %q: base fields are added automatically", i, field.Name)
		}
		if _, exists := seen[field.Name]; exists {
			return Resource{}, fmt.Errorf("fields[%d] %q: duplicate field", i, field.Name)
		}
		if _, exists := seenGoNames[field.GoName]; exists {
			return Resource{}, fmt.Errorf("fields[%d] %q: generated Go name %q collides with another field", i, field.Name, field.GoName)
		}
		seen[field.Name] = struct{}{}
		seenGoNames[field.GoName] = struct{}{}
		resource.Fields = append(resource.Fields, field)
	}
	return resource, nil
}

func normalizeField(raw rawField) (Field, error) {
	if !dbNamePattern.MatchString(raw.Name) {
		return Field{}, errors.New("name must be lowercase snake_case")
	}
	if _, ok := validTypes[raw.Type]; !ok {
		return Field{}, fmt.Errorf("unsupported type %q", raw.Type)
	}
	nullable := raw.Nullable != nil && *raw.Nullable
	hasDefault := raw.Default.Kind != 0
	var defaultValue any
	if hasDefault {
		if err := raw.Default.Decode(&defaultValue); err != nil {
			return Field{}, fmt.Errorf("decode default: %w", err)
		}
	}
	if raw.Required && nullable {
		return Field{}, errors.New("required and nullable cannot both be true")
	}
	if raw.Required && hasDefault {
		return Field{}, errors.New("required fields cannot declare a default")
	}
	if hasDefault && defaultValue == nil && !nullable {
		return Field{}, errors.New("a null default requires nullable: true")
	}
	if raw.MaxLength != nil {
		if *raw.MaxLength <= 0 {
			return Field{}, errors.New("max_length must be positive")
		}
		if raw.Type != TypeString && raw.Type != TypeText {
			return Field{}, errors.New("max_length is only valid for string and text")
		}
	}
	if raw.Searchable && raw.Type != TypeString && raw.Type != TypeText {
		return Field{}, errors.New("searchable is only valid for string and text")
	}
	if raw.Min != nil || raw.Max != nil {
		if !isNumeric(raw.Type) {
			return Field{}, errors.New("min and max are only valid for numeric fields")
		}
		if raw.Min != nil && !finite(*raw.Min) || raw.Max != nil && !finite(*raw.Max) {
			return Field{}, errors.New("min and max must be finite")
		}
		if raw.Min != nil && raw.Max != nil && *raw.Min > *raw.Max {
			return Field{}, errors.New("min cannot be greater than max")
		}
		if raw.Type == TypeInt32 || raw.Type == TypeInt64 {
			minimum, maximum := float64(math.MinInt64), float64(math.MaxInt64)
			if raw.Type == TypeInt32 {
				minimum, maximum = math.MinInt32, math.MaxInt32
			}
			if raw.Min != nil && *raw.Min > minimum {
				minimum = *raw.Min
			}
			if raw.Max != nil && *raw.Max < maximum {
				maximum = *raw.Max
			}
			if math.Ceil(minimum) > math.Floor(maximum) {
				return Field{}, errors.New("min and max contain no value representable by the integer type")
			}
		}
	}
	if len(raw.Enum) > 0 {
		if raw.Type != TypeString && raw.Type != TypeText {
			return Field{}, errors.New("enum is only valid for string and text")
		}
		seen := make(map[string]struct{}, len(raw.Enum))
		for _, value := range raw.Enum {
			if value == "" {
				return Field{}, errors.New("enum values cannot be empty")
			}
			if _, exists := seen[value]; exists {
				return Field{}, fmt.Errorf("duplicate enum value %q", value)
			}
			if raw.MaxLength != nil && utf8.RuneCountInString(value) > *raw.MaxLength {
				return Field{}, fmt.Errorf("enum value %q exceeds max_length", value)
			}
			seen[value] = struct{}{}
		}
	}
	if hasDefault && defaultValue != nil {
		if err := validateDefault(raw.Type, defaultValue); err != nil {
			return Field{}, err
		}
		if raw.Type == TypeDecimal {
			value, _ := decimal.NewFromString(fmt.Sprint(defaultValue))
			defaultValue = value.String()
		}
		if raw.MaxLength != nil {
			if value, ok := defaultValue.(string); ok && utf8.RuneCountInString(value) > *raw.MaxLength {
				return Field{}, errors.New("default exceeds max_length")
			}
		}
		if raw.Min != nil || raw.Max != nil {
			value, ok := constraintNumber(defaultValue)
			if !ok {
				return Field{}, errors.New("numeric default cannot be compared to min or max")
			}
			if raw.Min != nil && value < *raw.Min {
				return Field{}, errors.New("default is less than min")
			}
			if raw.Max != nil && value > *raw.Max {
				return Field{}, errors.New("default is greater than max")
			}
		}
		if len(raw.Enum) > 0 {
			value, ok := defaultValue.(string)
			if !ok || !slices.Contains(raw.Enum, value) {
				return Field{}, errors.New("default must be one of the enum values")
			}
		}
	}

	return Field{
		Name: raw.Name, GoName: toGoName(raw.Name), Column: raw.Name, Type: raw.Type,
		Required: raw.Required, Nullable: nullable, HasDefault: hasDefault, Default: defaultValue,
		Unique: raw.Unique, Index: raw.Index, Enum: slices.Clone(raw.Enum), Min: raw.Min,
		Max: raw.Max, MaxLength: raw.MaxLength, Searchable: raw.Searchable,
		Filterable: raw.Filterable, Sortable: raw.Sortable,
	}, nil
}

func validateDefault(fieldType FieldType, value any) error {
	switch fieldType {
	case TypeString, TypeText:
		if _, ok := value.(string); !ok {
			return errors.New("default must be a string")
		}
	case TypeBool:
		if _, ok := value.(bool); !ok {
			return errors.New("default must be a boolean")
		}
	case TypeInt32:
		n, ok := integer(value)
		if !ok || n < math.MinInt32 || n > math.MaxInt32 {
			return errors.New("default must be an int32")
		}
	case TypeInt64:
		if _, ok := integer(value); !ok {
			return errors.New("default must be an int64")
		}
	case TypeFloat64:
		if number, ok := number(value); !ok || !finite(number) {
			return errors.New("default must be numeric")
		}
	case TypeDecimal:
		if number, ok := number(value); ok {
			if !finite(number) {
				return errors.New("decimal default must be finite")
			}
		} else if text, ok := value.(string); !ok || !decimalPattern.MatchString(text) {
			return errors.New("decimal default must be a decimal number")
		} else if _, err := decimal.NewFromString(text); err != nil {
			return errors.New("decimal default must be a decimal number")
		}
	case TypeTime:
		text, ok := value.(string)
		if !ok {
			return errors.New("time default must be an RFC3339 string")
		}
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			return errors.New("time default must be an RFC3339 string")
		}
	case TypeUUID:
		text, ok := value.(string)
		if !ok || !uuidPattern.MatchString(text) {
			return errors.New("uuid default must be a canonical UUID string")
		}
	case TypeJSON:
		if _, err := json.Marshal(value); err != nil {
			return errors.New("json default must be JSON-serializable")
		}
	}
	return nil
}

func isNumeric(fieldType FieldType) bool {
	return fieldType == TypeInt32 || fieldType == TypeInt64 || fieldType == TypeFloat64 || fieldType == TypeDecimal
}

func integer(value any) (int64, bool) {
	switch n := value.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case uint64:
		if n <= math.MaxInt64 {
			return int64(n), true
		}
	}
	return 0, false
}

func number(value any) (float64, bool) {
	if n, ok := integer(value); ok {
		return float64(n), true
	}
	n, ok := value.(float64)
	return n, ok
}

func constraintNumber(value any) (float64, bool) {
	if value, ok := number(value); ok {
		return value, true
	}
	if text, ok := value.(string); ok {
		value, err := strconv.ParseFloat(text, 64)
		return value, err == nil
	}
	return 0, false
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func toGoName(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		switch strings.ToLower(part) {
		case "id":
			parts[i] = "ID"
		case "api":
			parts[i] = "API"
		case "url":
			parts[i] = "URL"
		case "uuid":
			parts[i] = "UUID"
		case "json":
			parts[i] = "JSON"
		default:
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

// Fingerprint is a stable digest of the normalized specification.
func (r Resource) Fingerprint() string {
	data, _ := json.Marshal(r)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
