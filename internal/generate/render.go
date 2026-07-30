package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"math"
	"strconv"
	"strings"
	"text/template"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"github.com/shopspring/decimal"
)

type resourceData struct {
	Module   string
	Resource spec.Resource
}

type allResourcesData struct {
	Module    string
	Resources []spec.Resource
}

func renderGenerated(modulePath string, resources []spec.Resource) (map[string][]byte, error) {
	files := make(map[string][]byte)
	resourceTemplates := []struct {
		name string
		body string
	}{
		{"model_gen.go", modelTemplate},
		{"dto_gen.go", dtoTemplate},
		{"store_gen.go", storeTemplate},
		{"http_gen.go", httpTemplate},
		{"contract_gen_test.go", contractTemplate},
	}
	for _, resource := range resources {
		data := resourceData{Module: modulePath, Resource: resource}
		for _, item := range resourceTemplates {
			output, err := executeGoTemplate(item.name, item.body, data)
			if err != nil {
				return nil, fmt.Errorf("render %s/%s: %w", resource.Package, item.name, err)
			}
			files["internal/resources/"+resource.Package+"/"+item.name] = output
		}
	}
	all := allResourcesData{Module: modulePath, Resources: resources}
	for name, body := range map[string]string{
		"internal/generated/register_gen.go": registerTemplate,
		"tools/gormschema/main_gen.go":       gormSchemaTemplate,
		"openapi/embed_gen.go":               openAPIEmbedTemplate,
	} {
		output, err := executeGoTemplate(name, body, all)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", name, err)
		}
		files[name] = output
	}
	document := buildOpenAPI(resources)
	data, err := marshalJSON(document)
	if err != nil {
		return nil, err
	}
	if err := validateOpenAPI(data); err != nil {
		return nil, err
	}
	files["openapi/openapi_gen.json"] = data
	return files, nil
}

func executeGoTemplate(name, body string, data any) ([]byte, error) {
	functions := template.FuncMap{
		"baseType":              baseGoType,
		"modelType":             modelGoType,
		"modelImports":          modelImports,
		"dtoImports":            dtoImports,
		"hasDTOImports":         func(r spec.Resource) bool { return dtoImports(r) != "" },
		"gormTag":               gormTag,
		"fieldStructTag":        fieldStructTag,
		"quote":                 strconv.Quote,
		"lower":                 strings.ToLower,
		"searchColumns":         searchColumns,
		"searchSQL":             buildSearchSQL,
		"sqlColumn":             sqlColumn,
		"filterFields":          filterFields,
		"sortableFields":        sortableFields,
		"hasFields":             func(r spec.Resource) bool { return len(r.Fields) > 0 },
		"hasDefaults":           hasDefaults,
		"defaultsJSON":          defaultsJSON,
		"defaultZeroFields":     defaultZeroFields,
		"nullableDefaultFields": nullableDefaultFields,
		"timeFields":            timeFields,
		"optionalJSONFields":    optionalJSONFields,
		"zeroExpectedJSON":      zeroExpectedJSON,
		"firstNullable":         firstNullable,
		"firstUnique":           firstUnique,
		"firstRequired":         firstRequired,
		"firstZero":             firstZero,
		"firstSearch":           firstSearch,
		"firstFilter":           firstFilter,
		"sampleGo":              sampleGoValue,
		"zeroGo":                zeroGoValue,
		"filterRaw":             filterRawValue,
		"searchRaw":             searchRawValue,
		"minValue":              func(f spec.Field) string { return f.Min.String() },
		"maxValue":              func(f spec.Field) string { return f.Max.String() },
		"maxLength":             func(f spec.Field) int { return *f.MaxLength },
	}
	tmpl, err := template.New(name).Funcs(functions).Option("missingkey=error").Parse(body)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, data); err != nil {
		return nil, err
	}
	formatted, err := format.Source(output.Bytes())
	if err != nil {
		return nil, fmt.Errorf("go/format: %w\n%s", err, output.String())
	}
	return formatted, nil
}

func baseGoType(field spec.Field) string {
	switch field.Type {
	case spec.TypeString, spec.TypeText:
		return "string"
	case spec.TypeBool:
		return "bool"
	case spec.TypeInt32:
		return "int32"
	case spec.TypeInt64:
		return "int64"
	case spec.TypeFloat64:
		return "float64"
	case spec.TypeDecimal:
		return "decimal.Decimal"
	case spec.TypeTime:
		return "time.Time"
	case spec.TypeUUID:
		return "uuid.UUID"
	case spec.TypeJSON:
		return "datatypes.JSON"
	default:
		panic("unsupported field type")
	}
}

func modelGoType(field spec.Field) string {
	typ := baseGoType(field)
	if field.Nullable {
		typ = "*" + typ
	}
	return typ
}

func modelImports(resource spec.Resource) string {
	imports := []string{"\"time\"", "\"gorm.io/gorm\""}
	if hasType(resource, spec.TypeUUID) {
		imports = append(imports, "\"github.com/google/uuid\"")
	}
	if hasType(resource, spec.TypeDecimal) {
		imports = append(imports, "\"github.com/shopspring/decimal\"")
	}
	if hasType(resource, spec.TypeJSON) {
		imports = append(imports, "\"gorm.io/datatypes\"")
	}
	return strings.Join(imports, "\n")
}

func dtoImports(resource spec.Resource) string {
	var imports []string
	if hasType(resource, spec.TypeTime) {
		imports = append(imports, "\"time\"")
	}
	if hasType(resource, spec.TypeUUID) {
		imports = append(imports, "\"github.com/google/uuid\"")
	}
	if hasType(resource, spec.TypeDecimal) {
		imports = append(imports, "\"github.com/shopspring/decimal\"")
	}
	if hasType(resource, spec.TypeJSON) {
		imports = append(imports, "\"gorm.io/datatypes\"")
	}
	return strings.Join(imports, "\n")
}

func hasType(resource spec.Resource, fieldType spec.FieldType) bool {
	for _, field := range resource.Fields {
		if field.Type == fieldType {
			return true
		}
	}
	return false
}

func gormTag(field spec.Field) string {
	parts := []string{"column:" + field.Column}
	switch field.Type {
	case spec.TypeString:
		if field.MaxLength != nil {
			parts = append(parts, fmt.Sprintf("type:varchar(%d)", *field.MaxLength))
		}
	case spec.TypeText:
		parts = append(parts, "type:text")
	case spec.TypeDecimal:
		parts = append(parts, "type:numeric")
	case spec.TypeUUID:
		parts = append(parts, "type:uuid")
	case spec.TypeJSON:
		parts = append(parts, "type:jsonb")
	}
	if !field.Nullable {
		parts = append(parts, "not null")
	}
	if field.Unique {
		parts = append(parts, "uniqueIndex")
	} else if field.Index {
		parts = append(parts, "index")
	}
	if field.HasDefault && field.Default != nil {
		parts = append(parts, "default:"+defaultTagValue(field.Type, field.Default))
	}
	return strings.Join(parts, ";")
}

func fieldStructTag(field spec.Field) string {
	return strconv.Quote(fmt.Sprintf(`json:"%s" gorm:"%s"`, field.Name, gormTag(field)))
}

func defaultTagValue(fieldType spec.FieldType, value any) string {
	if fieldType == spec.TypeJSON {
		data, _ := json.Marshal(value)
		return escapeGORMTag("'" + strings.ReplaceAll(string(data), "'", "''") + "'")
	}
	if fieldType == spec.TypeString || fieldType == spec.TypeText {
		text, _ := value.(string)
		return escapeGORMTag("('" + strings.ReplaceAll(text, "'", "''") + "')")
	}
	var rendered string
	switch value := value.(type) {
	case string:
		rendered = value
	case bool:
		rendered = strconv.FormatBool(value)
	case int:
		rendered = strconv.Itoa(value)
	case int64:
		rendered = strconv.FormatInt(value, 10)
	case float64:
		rendered = strconv.FormatFloat(value, 'g', -1, 64)
	default:
		data, _ := json.Marshal(value)
		rendered = string(data)
	}
	return escapeGORMTag(rendered)
}

func escapeGORMTag(value string) string {
	value = strings.ReplaceAll(value, ";", `\;`)
	quoted := strconv.Quote(value)
	return strings.TrimSuffix(strings.TrimPrefix(quoted, `"`), `"`)
}

func searchColumns(resource spec.Resource) []string {
	var values []string
	for _, field := range resource.Fields {
		if field.Searchable {
			values = append(values, field.Column)
		}
	}
	return values
}

func filterFields(resource spec.Resource) []spec.Field {
	var values []spec.Field
	for _, field := range resource.Fields {
		if field.Filterable {
			values = append(values, field)
		}
	}
	return values
}

func sortableFields(resource spec.Resource) []spec.Field {
	var values []spec.Field
	for _, field := range resource.Fields {
		if field.Sortable {
			values = append(values, field)
		}
	}
	return values
}

func firstMatching(resource spec.Resource, match func(spec.Field) bool) *spec.Field {
	for i := range resource.Fields {
		if match(resource.Fields[i]) {
			field := resource.Fields[i]
			return &field
		}
	}
	return nil
}

func hasDefaults(resource spec.Resource) bool {
	for _, field := range resource.Fields {
		if field.HasDefault {
			return true
		}
	}
	return false
}

func zeroAllowed(field spec.Field) bool {
	switch field.Type {
	case spec.TypeBool:
		return true
	case spec.TypeInt32, spec.TypeInt64, spec.TypeFloat64, spec.TypeDecimal:
		return numericZeroAllowed(field)
	case spec.TypeString, spec.TypeText:
		return len(field.Enum) == 0
	default:
		return false
	}
}

func defaultZeroFields(resource spec.Resource) []spec.Field {
	var fields []spec.Field
	for _, field := range resource.Fields {
		if field.HasDefault && field.Default != nil && zeroAllowed(field) {
			fields = append(fields, field)
		}
	}
	return fields
}

func nullableDefaultFields(resource spec.Resource) []spec.Field {
	var fields []spec.Field
	for _, field := range resource.Fields {
		if field.Nullable && field.HasDefault && field.Default != nil {
			fields = append(fields, field)
		}
	}
	return fields
}

func timeFields(resource spec.Resource) []spec.Field {
	var fields []spec.Field
	for _, field := range resource.Fields {
		if field.Type == spec.TypeTime {
			fields = append(fields, field)
		}
	}
	return fields
}

func optionalJSONFields(resource spec.Resource) []spec.Field {
	var fields []spec.Field
	for _, field := range resource.Fields {
		if field.Type == spec.TypeJSON && !field.Required && !field.Nullable && !field.HasDefault {
			fields = append(fields, field)
		}
	}
	return fields
}

func zeroExpectedJSON(field spec.Field) string {
	var value any
	switch field.Type {
	case spec.TypeBool:
		value = false
	case spec.TypeInt32, spec.TypeInt64, spec.TypeFloat64:
		value = 0
	case spec.TypeDecimal:
		value = "0"
	case spec.TypeString, spec.TypeText:
		value = ""
	default:
		panic("field has no zero JSON value")
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func defaultsJSON(resource spec.Resource) string {
	values := make(map[string]any)
	for _, field := range resource.Fields {
		if !field.HasDefault {
			continue
		}
		value := field.Default
		if field.Type == spec.TypeDecimal && value != nil {
			value = fmt.Sprint(value)
		}
		values[field.Name] = value
	}
	data, _ := json.Marshal(values)
	return string(data)
}

func firstNullable(resource spec.Resource) *spec.Field {
	return firstMatching(resource, func(field spec.Field) bool { return field.Nullable })
}

func firstUnique(resource spec.Resource) *spec.Field {
	return firstMatching(resource, func(field spec.Field) bool { return field.Unique })
}

func firstRequired(resource spec.Resource) *spec.Field {
	return firstMatching(resource, func(field spec.Field) bool { return field.Required })
}

func firstSearch(resource spec.Resource) *spec.Field {
	return firstMatching(resource, func(field spec.Field) bool { return field.Searchable })
}

func firstFilter(resource spec.Resource) *spec.Field {
	return firstMatching(resource, func(field spec.Field) bool { return field.Filterable })
}

func firstZero(resource spec.Resource) *spec.Field {
	return firstMatching(resource, func(field spec.Field) bool {
		switch field.Type {
		case spec.TypeBool:
			return true
		case spec.TypeInt32, spec.TypeInt64, spec.TypeFloat64, spec.TypeDecimal:
			return numericZeroAllowed(field)
		case spec.TypeString, spec.TypeText:
			return len(field.Enum) == 0
		default:
			return false
		}
	})
}

func sampleGoValue(field spec.Field) string {
	if len(field.Enum) > 0 {
		return strconv.Quote(field.Enum[0])
	}
	switch field.Type {
	case spec.TypeString, spec.TypeText:
		return strconv.Quote("x")
	case spec.TypeBool:
		return "true"
	case spec.TypeInt32:
		return "int32(" + sampleInteger(field) + ")"
	case spec.TypeInt64:
		return "int64(" + sampleInteger(field) + ")"
	case spec.TypeFloat64:
		return sampleFloatNumber(field)
	case spec.TypeDecimal:
		return strconv.Quote(sampleNumber(field))
	case spec.TypeTime:
		return strconv.Quote("2026-01-02T11:04:05+08:00")
	case spec.TypeUUID:
		return strconv.Quote("550e8400-e29b-41d4-a716-446655440000")
	case spec.TypeJSON:
		return `map[string]any{"sample": true}`
	default:
		panic("unsupported sample type")
	}
}

func zeroGoValue(field spec.Field) string {
	switch field.Type {
	case spec.TypeBool:
		return "false"
	case spec.TypeInt32, spec.TypeInt64, spec.TypeFloat64:
		return "0"
	case spec.TypeDecimal:
		return strconv.Quote("0")
	case spec.TypeString, spec.TypeText:
		return `""`
	default:
		panic("field has no test zero value")
	}
}

func filterRawValue(field spec.Field) string {
	if len(field.Enum) > 0 {
		return field.Enum[0]
	}
	switch field.Type {
	case spec.TypeString, spec.TypeText:
		return "x"
	case spec.TypeBool:
		return "true"
	case spec.TypeInt32, spec.TypeInt64:
		return sampleInteger(field)
	case spec.TypeFloat64:
		return sampleFloatNumber(field)
	case spec.TypeDecimal:
		return sampleNumber(field)
	case spec.TypeTime:
		return "2026-01-02T03:04:05Z"
	case spec.TypeUUID:
		return "550e8400-e29b-41d4-a716-446655440000"
	case spec.TypeJSON:
		return `{"sample":true}`
	default:
		panic("unsupported filter type")
	}
}

func numericZeroAllowed(field spec.Field) bool {
	return (field.Min == nil || !field.Min.Decimal().GreaterThan(decimal.Zero)) &&
		(field.Max == nil || !field.Max.Decimal().LessThan(decimal.Zero))
}

func sampleInteger(field spec.Field) string {
	value := decimal.NewFromInt(42)
	if field.Min != nil && value.LessThan(field.Min.Decimal()) {
		value = field.Min.Decimal().Ceil()
	}
	if field.Max != nil && value.GreaterThan(field.Max.Decimal()) {
		value = field.Max.Decimal().Floor()
	}
	return value.StringFixed(0)
}

func sampleNumber(field spec.Field) string {
	value := decimal.RequireFromString("12.5")
	if field.Min != nil && value.LessThan(field.Min.Decimal()) {
		value = field.Min.Decimal()
	}
	if field.Max != nil && value.GreaterThan(field.Max.Decimal()) {
		value = field.Max.Decimal()
	}
	return value.String()
}

func sampleFloatNumber(field spec.Field) string {
	value := 12.5
	if field.Min != nil && decimal.NewFromFloat(value).LessThan(field.Min.Decimal()) {
		value = float64AtLeast(field.Min.Decimal())
	}
	if field.Max != nil && decimal.NewFromFloat(value).GreaterThan(field.Max.Decimal()) {
		value = float64AtMost(field.Max.Decimal())
	}
	return strconv.FormatFloat(value, 'g', -1, 64)
}

func float64AtLeast(value decimal.Decimal) float64 {
	number, _ := value.Float64()
	if decimal.NewFromFloat(number).LessThan(value) {
		number = math.Nextafter(number, math.Inf(1))
	}
	return number
}

func float64AtMost(value decimal.Decimal) float64 {
	number, _ := value.Float64()
	if decimal.NewFromFloat(number).GreaterThan(value) {
		number = math.Nextafter(number, math.Inf(-1))
	}
	return number
}

func searchRawValue(field spec.Field) string {
	value := filterRawValue(field)
	if len(value) > 3 {
		return value[:3]
	}
	return value
}

const modelTemplate = `// Code generated by gobackend; DO NOT EDIT.

package {{.Resource.Package}}

import (
{{modelImports .Resource}}
)

type {{.Resource.Name}} struct {
	ID int64 ` + "`json:\"id\" gorm:\"primaryKey;autoIncrement\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\" gorm:\"not null;autoCreateTime\"`" + `
	UpdatedAt time.Time ` + "`json:\"updated_at\" gorm:\"not null;autoUpdateTime\"`" + `
{{range .Resource.Fields}}	{{.GoName}} {{modelType .}} {{fieldStructTag .}}
{{end}}}

func ({{.Resource.Name}}) TableName() string { return {{quote .Resource.Table}} }

func (value *{{.Resource.Name}}) AfterFind(_ *gorm.DB) error {
	value.CreatedAt = value.CreatedAt.UTC()
	value.UpdatedAt = value.UpdatedAt.UTC()
{{range timeFields .Resource}}{{if .Nullable}}	if value.{{.GoName}} != nil { normalized := value.{{.GoName}}.UTC(); value.{{.GoName}} = &normalized }
{{else}}	value.{{.GoName}} = value.{{.GoName}}.UTC()
{{end}}{{end}}	return nil
}
`

const dtoTemplate = `// Code generated by gobackend; DO NOT EDIT.

package {{.Resource.Package}}

import (
{{dtoImports .Resource}}

{{if hasFields .Resource}}	"{{.Module}}/internal/platform/optional"
{{end}}	"{{.Module}}/internal/platform/validation"
)

type Create{{.Resource.Name}}Input struct {
{{range .Resource.Fields}}	{{.GoName}} optional.Field[{{baseType .}}] ` + "`json:\"{{.Name}}\"`" + `
{{end}}}

type Update{{.Resource.Name}}Input struct {
{{range .Resource.Fields}}	{{.GoName}} optional.Field[{{baseType .}}] ` + "`json:\"{{.Name}}\"`" + `
{{end}}}

func validateCreate(input Create{{.Resource.Name}}Input) validation.Details {
	details := validation.Details{}
{{range .Resource.Fields}}	validation.Presence(details, {{quote .Name}}, input.{{.GoName}}.IsSet(), input.{{.GoName}}.IsNull(), {{.Required}}, {{.Nullable}})
{{if .MaxLength}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.MaxLength(details, {{quote .Name}}, value, {{maxLength .}}) }
{{end}}{{if .Enum}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.OneOf(details, {{quote .Name}}, value, []string{ {{range .Enum}}{{quote .}},{{end}} }) }
{{end}}{{if .Min}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.Min(details, {{quote .Name}}, value, {{quote (minValue .)}}) }
{{end}}{{if .Max}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.Max(details, {{quote .Name}}, value, {{quote (maxValue .)}}) }
{{end}}{{end}}	return details
}

func validateUpdate(input Update{{.Resource.Name}}Input) validation.Details {
	details := validation.Details{}
{{range .Resource.Fields}}	validation.Presence(details, {{quote .Name}}, input.{{.GoName}}.IsSet(), input.{{.GoName}}.IsNull(), false, {{.Nullable}})
{{if .MaxLength}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.MaxLength(details, {{quote .Name}}, value, {{maxLength .}}) }
{{end}}{{if .Enum}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.OneOf(details, {{quote .Name}}, value, []string{ {{range .Enum}}{{quote .}},{{end}} }) }
{{end}}{{if .Min}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.Min(details, {{quote .Name}}, value, {{quote (minValue .)}}) }
{{end}}{{if .Max}}	if value, ok := input.{{.GoName}}.Value(); ok { validation.Max(details, {{quote .Name}}, value, {{quote (maxValue .)}}) }
{{end}}{{end}}	return details
}

func createValues(input Create{{.Resource.Name}}Input) (map[string]any, validation.Details) {
	details := validateCreate(input)
	if len(details) > 0 {
		return nil, details
	}
	zeroModel := {{.Resource.Name}}{}
	_ = zeroModel
	values := map[string]any{}
{{range .Resource.Fields}}{{if not .HasDefault}}{{if and (eq .Type "json") (not .Nullable)}}	values[{{quote .Column}}] = []byte("{}")
{{else}}	values[{{quote .Column}}] = zeroModel.{{.GoName}}
{{end}}{{end}}{{end}}{{range .Resource.Fields}}	if input.{{.GoName}}.IsNull() {
		values[{{quote .Column}}] = nil
	} else if value, ok := input.{{.GoName}}.Value(); ok {
{{if eq .Type "time"}}		value = value.UTC()
{{end}}		values[{{quote .Column}}] = value
	}
{{end}}	return values, nil
}

func updateValues(input Update{{.Resource.Name}}Input) (map[string]any, validation.Details) {
	details := validateUpdate(input)
	if len(details) > 0 {
		return nil, details
	}
	values := map[string]any{}
{{range .Resource.Fields}}	if input.{{.GoName}}.IsSet() {
		if input.{{.GoName}}.IsNull() {
			values[{{quote .Column}}] = nil
		} else if value, ok := input.{{.GoName}}.Value(); ok {
{{if eq .Type "time"}}			value = value.UTC()
{{end}}			values[{{quote .Column}}] = value
		}
	}
{{end}}	return values, nil
}
`

const storeTemplate = `// Code generated by gobackend; DO NOT EDIT.

package {{.Resource.Package}}

import (
	"context"
	"fmt"
	"strings"
	"time"

	"{{.Module}}/internal/resources/{{.Resource.Package}}/gormgen"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type filters struct {
	page int
	pageSize int
	sort string
	query string
	exact map[string]any
}

type store struct {
	db *gorm.DB
}

func (s store) list(ctx context.Context, filters filters) ([]{{.Resource.Name}}, int64, error) {
	db := s.db.WithContext(ctx).Model(&{{.Resource.Name}}{})
{{if searchColumns .Resource}}	if filters.query != "" {
		pattern := "%" + strings.ToLower(filters.query) + "%"
		db = db.Where({{quote (searchSQL .Resource)}}{{range searchColumns .Resource}}, pattern{{end}})
	}
{{end}}{{range filterFields .Resource}}	if value, ok := filters.exact[{{quote .Name}}]; ok {
		db = db.Where(clause.Eq{Column: gormgen.{{$.Resource.Name}}.{{.GoName}}.Column(), Value: value})
	}
{{end}}	var total int64
	if err := db.Count(&total).Error; err != nil { return nil, 0, err }
	orders := []clause.OrderByColumn{gormgen.{{.Resource.Name}}.CreatedAt.Desc(), gormgen.{{.Resource.Name}}.ID.Desc()}
	switch filters.sort {
	case "id": orders = []clause.OrderByColumn{gormgen.{{.Resource.Name}}.ID.Asc()}
	case "-id": orders = []clause.OrderByColumn{gormgen.{{.Resource.Name}}.ID.Desc()}
	case "created_at": orders = []clause.OrderByColumn{gormgen.{{.Resource.Name}}.CreatedAt.Asc()}
	case "-created_at": orders = []clause.OrderByColumn{gormgen.{{.Resource.Name}}.CreatedAt.Desc()}
{{range sortableFields .Resource}}	case {{quote .Name}}: orders = []clause.OrderByColumn{gormgen.{{$.Resource.Name}}.{{.GoName}}.Asc()}
	case {{quote (printf "-%s" .Name)}}: orders = []clause.OrderByColumn{gormgen.{{$.Resource.Name}}.{{.GoName}}.Desc()}
{{end}}	}
	for _, order := range orders { db = db.Order(order) }
	var items []{{.Resource.Name}}
	err := db.Offset((filters.page-1)*filters.pageSize).Limit(filters.pageSize).Find(&items).Error
	return items, total, err
}

func (s store) get(ctx context.Context, id int64) ({{.Resource.Name}}, error) {
	return gorm.G[{{.Resource.Name}}](s.db).Where("id = ?", id).First(ctx)
}

func (s store) create(ctx context.Context, values map[string]any) ({{.Resource.Name}}, error) {
	now := time.Now().UTC()
	columns := []string{"\"created_at\"", "\"updated_at\""}
	arguments := []any{now, now}
{{range .Resource.Fields}}	if value, present := values[{{quote .Column}}]; present { columns = append(columns, {{quote (sqlColumn .Column)}}); arguments = append(arguments, value) }
{{end}}	placeholders := make([]string, len(columns))
	for index := range placeholders { placeholders[index] = "?" }
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING \"id\"", {{quote (sqlColumn .Resource.Table)}}, strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	var created {{.Resource.Name}}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id int64
		if err := tx.Raw(query, arguments...).Scan(&id).Error; err != nil { return translateDatabaseError(tx, err) }
		item, err := (store{db: tx}).get(ctx, id)
		if err != nil { return err }
		created = item
		return nil
	})
	return created, err
}

func (s store) update(ctx context.Context, id int64, values map[string]any) ({{.Resource.Name}}, error) {
	result := s.db.WithContext(ctx).Model(new({{.Resource.Name}})).Where("id = ?", id).Updates(values)
	if result.Error != nil { return {{.Resource.Name}}{}, translateDatabaseError(s.db, result.Error) }
	if result.RowsAffected == 0 { return {{.Resource.Name}}{}, gorm.ErrRecordNotFound }
	return s.get(ctx, id)
}

func (s store) delete(ctx context.Context, id int64) error {
	rows, err := gorm.G[{{.Resource.Name}}](s.db).Where("id = ?", id).Delete(ctx)
	if err != nil { return translateDatabaseError(s.db, err) }
	if rows == 0 { return gorm.ErrRecordNotFound }
	return nil
}

func translateDatabaseError(db *gorm.DB, err error) error {
	if translator, ok := db.Dialector.(gorm.ErrorTranslator); ok { return translator.Translate(err) }
	return err
}
`

const httpTemplate = `// Code generated by gobackend; DO NOT EDIT.

package {{.Resource.Package}}

import (
	"net/http"
	"strconv"
	"strings"

	"{{.Module}}/internal/platform/apperror"
	"{{.Module}}/internal/platform/httpx"
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
)

type handler struct { store store }

func Register(group *echo.Group, db *gorm.DB) {
	h := handler{store: store{db: db}}
	resource := group.Group({{quote .Resource.Route}})
	resource.GET("", h.list)
	resource.GET("/:id", h.get)
	resource.POST("", h.create)
	resource.PATCH("/:id", h.update)
	resource.DELETE("/:id", h.delete)
}

func (h handler) list(c *echo.Context) error {
	page, pageSize, err := httpx.ParsePage(c.QueryParam("page"), c.QueryParam("page_size"))
	if err != nil { return httpx.WriteError(c, err) }
	sortValue := c.QueryParam("sort")
	switch strings.TrimPrefix(sortValue, "-") {
	case "", "id", "created_at"{{range sortableFields .Resource}}, {{quote .Name}}{{end}}:
	default: return httpx.WriteError(c, apperror.BadRequest("invalid_sort", "sort is not allowed"))
	}
	exact := map[string]any{}
{{range filterFields .Resource}}	if values, present := c.QueryParams()[{{quote .Name}}]; present { if len(values) != 1 { return httpx.WriteError(c, apperror.BadRequest("invalid_filter", {{quote (printf "%s filter must be provided once" .Name)}})) }; value, parseErr := httpx.ParseScalar(values[0], {{quote (printf "%s" .Type)}}); if parseErr != nil { return httpx.WriteError(c, apperror.BadRequest("invalid_filter", {{quote (printf "invalid %s filter" .Name)}})) }; exact[{{quote .Name}}] = value }
{{end}}	items, total, err := h.store.list(c.Request().Context(), filters{page: page, pageSize: pageSize, sort: sortValue, query: c.QueryParam("q"), exact: exact})
	if err != nil { return httpx.WriteError(c, apperror.FromDatabase(err)) }
	return httpx.WriteData(c, http.StatusOK, httpx.NewPage(items, page, pageSize, total))
}

func (h handler) get(c *echo.Context) error {
	id, err := parseID(c.Param("id")); if err != nil { return httpx.WriteError(c, err) }
	item, err := h.store.get(c.Request().Context(), id)
	if err != nil { return httpx.WriteError(c, apperror.FromDatabase(err)) }
	return httpx.WriteData(c, http.StatusOK, item)
}

func (h handler) create(c *echo.Context) error {
	var input Create{{.Resource.Name}}Input
	if err := httpx.DecodeJSON(c.Request(), &input); err != nil { return httpx.WriteError(c, apperror.BadRequest("invalid_json", "request body must be valid JSON")) }
	values, details := createValues(input)
	if len(details) > 0 { return httpx.WriteError(c, apperror.Validation(details)) }
	item, err := h.store.create(c.Request().Context(), values)
	if err != nil { return httpx.WriteError(c, apperror.FromDatabase(err)) }
	return httpx.WriteData(c, http.StatusCreated, item)
}

func (h handler) update(c *echo.Context) error {
	id, err := parseID(c.Param("id")); if err != nil { return httpx.WriteError(c, err) }
	var input Update{{.Resource.Name}}Input
	if err := httpx.DecodeJSON(c.Request(), &input); err != nil { return httpx.WriteError(c, apperror.BadRequest("invalid_json", "request body must be valid JSON")) }
	values, details := updateValues(input)
	if len(details) > 0 { return httpx.WriteError(c, apperror.Validation(details)) }
	if len(values) == 0 { return httpx.WriteError(c, apperror.BadRequest("empty_update", "at least one field must be provided")) }
	item, err := h.store.update(c.Request().Context(), id, values)
	if err != nil { return httpx.WriteError(c, apperror.FromDatabase(err)) }
	return httpx.WriteData(c, http.StatusOK, item)
}

func (h handler) delete(c *echo.Context) error {
	id, err := parseID(c.Param("id")); if err != nil { return httpx.WriteError(c, err) }
	if err := h.store.delete(c.Request().Context(), id); err != nil { return httpx.WriteError(c, apperror.FromDatabase(err)) }
	c.Response().WriteHeader(http.StatusNoContent)
	return nil
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 { return 0, apperror.BadRequest("invalid_id", "id must be a positive integer") }
	return id, nil
}
`

const contractTemplate = `// Code generated by gobackend; DO NOT EDIT.

package {{.Resource.Package}}

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
{{if hasDefaults .Resource}}	"reflect"
{{end}}	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/libtnb/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestGenerated{{.Resource.Name}}CRUDContract(t *testing.T) {
	config := &gorm.Config{TranslateError: true, NowFunc: func() time.Time { return time.Now().UTC() }, Logger: gormlogger.Default.LogMode(gormlogger.Silent)}
	var db *gorm.DB
	var err error
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), config)
	} else {
		db, err = gorm.Open(sqlite.Open(":memory:"), config)
		if err == nil { err = db.AutoMigrate(&{{.Resource.Name}}{}) }
	}
	if err != nil { t.Fatal(err) }
	e := echo.New()
	Register(e.Group("/api/v1"), db)
	basePath := "/api/v1{{.Resource.Route}}"
	escapeQuery := url.QueryEscape
	payload := map[string]any{
{{range .Resource.Fields}}		{{quote .Name}}: {{sampleGo .}},
{{end}}	}

	// Create.
	createdResponse := performJSON(t, e, http.MethodPost, basePath, payload)
	requireStatus(t, createdResponse, http.StatusCreated)
	var created struct { Data {{.Resource.Name}} ` + "`json:\"data\"`" + ` }
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil { t.Fatal(err) }
	if created.Data.ID <= 0 || created.Data.CreatedAt.Location() != time.UTC || created.Data.UpdatedAt.Location() != time.UTC { t.Fatalf("invalid base fields: %#v", created.Data) }
{{if timeFields .Resource}}	var createdMap struct { Data map[string]any ` + "`json:\"data\"`" + ` }
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &createdMap); err != nil { t.Fatal(err) }
{{range timeFields .Resource}}	if got := createdMap.Data[{{quote .Name}}]; got != "2026-01-02T03:04:05Z" { t.Fatalf("UTC {{.Name}} = %#v", got) }
{{end}}{{end}}

	// base field injection is rejected rather than mass-assigned.
	injected := map[string]any{"id": 999, "created_at": "2000-01-01T00:00:00Z"}
{{range .Resource.Fields}}	injected[{{quote .Name}}] = {{sampleGo .}}
{{end}}	requireStatus(t, performJSON(t, e, http.MethodPost, basePath, injected), http.StatusBadRequest)

{{with firstUnique .Resource}}	// A unique conflict maps to 409 without exposing the database error.
	conflict := performJSON(t, e, http.MethodPost, basePath, payload)
	requireStatus(t, conflict, http.StatusConflict)
	if strings.Contains(strings.ToLower(conflict.Body.String()), "unique constraint") { t.Fatal("unique conflict leaked database details") }
{{end}}
	// Get.
	detailPath := basePath + "/" + fmtID(created.Data.ID)
	requireStatus(t, performJSON(t, e, http.MethodGet, detailPath, nil), http.StatusOK)

	// List, pagination, and sort.
	list := performJSON(t, e, http.MethodGet, basePath+"?page=1&page_size=1&sort="+escapeQuery("-id"), nil)
	requireStatus(t, list, http.StatusOK)
	requireTotal(t, list, 1)
	if !bytes.Contains(list.Body.Bytes(), []byte(` + "`\"total_pages\":1`" + `)) { t.Fatalf("list envelope = %s", list.Body.String()) }
	requireStatus(t, performJSON(t, e, http.MethodGet, basePath+"?page_size=101", nil), http.StatusBadRequest)
	requireStatus(t, performJSON(t, e, http.MethodGet, basePath+"?page=9223372036854775807&page_size=100", nil), http.StatusBadRequest)
	requireStatus(t, performJSON(t, e, http.MethodGet, basePath+"?sort=not_allowed", nil), http.StatusBadRequest)
{{with firstSearch .Resource}}	search := performJSON(t, e, http.MethodGet, basePath+"?q="+escapeQuery({{quote (searchRaw .)}}), nil)
	requireStatus(t, search, http.StatusOK)
	requireTotal(t, search, 1)
{{end}}{{with firstFilter .Resource}}	filter := performJSON(t, e, http.MethodGet, basePath+"?{{.Name}}="+escapeQuery({{quote (filterRaw .)}}), nil)
	requireStatus(t, filter, http.StatusOK)
	requireTotal(t, filter, 1)
{{end}}{{with firstZero .Resource}}
	// A zero-value PATCH remains present (false, 0, or empty string).
	zeroPatch := performJSON(t, e, http.MethodPatch, detailPath, map[string]any{ {{quote .Name}}: {{zeroGo .}} })
	requireStatus(t, zeroPatch, http.StatusOK)
{{end}}{{with firstNullable .Resource}}
	// An explicit null is different from an omitted PATCH field.
	nullPatch := performJSON(t, e, http.MethodPatch, detailPath, map[string]any{ {{quote .Name}}: nil })
	requireStatus(t, nullPatch, http.StatusOK)
	var nullData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
	if err := json.Unmarshal(nullPatch.Body.Bytes(), &nullData); err != nil { t.Fatal(err) }
	if value, exists := nullData.Data[{{quote .Name}}]; !exists || value != nil { t.Fatalf("explicit null was not persisted: %v", nullData.Data) }
{{end}}
	{{range timeFields .Resource}}{
		// Business time values are normalized to UTC on PATCH.
		timePatch := performJSON(t, e, http.MethodPatch, detailPath, map[string]any{ {{quote .Name}}: "2026-02-03T12:05:06+09:00" })
		requireStatus(t, timePatch, http.StatusOK)
		var timeData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
		if err := json.Unmarshal(timePatch.Body.Bytes(), &timeData); err != nil { t.Fatal(err) }
		if got := timeData.Data[{{quote .Name}}]; got != "2026-02-03T03:05:06Z" { t.Fatalf("UTC PATCH {{.Name}} = %#v", got) }
	}
{{end}}
	// An empty update is rejected.
	requireStatus(t, performJSON(t, e, http.MethodPatch, detailPath, map[string]any{}), http.StatusBadRequest)
{{with firstRequired .Resource}}
	// Missing required input is a validation error.
	requireStatus(t, performJSON(t, e, http.MethodPost, basePath, map[string]any{}), http.StatusUnprocessableEntity)
{{end}}
	// Delete, then confirm 404.
	requireStatus(t, performJSON(t, e, http.MethodDelete, detailPath, nil), http.StatusNoContent)
	requireStatus(t, performJSON(t, e, http.MethodGet, detailPath, nil), http.StatusNotFound)
{{if hasDefaults .Resource}}
	// Database defaults are applied when optional fields are omitted.
	defaultPayload := map[string]any{
{{range .Resource.Fields}}{{if .Required}}		{{quote .Name}}: {{sampleGo .}},
{{end}}{{end}}	}
	defaultResponse := performJSON(t, e, http.MethodPost, basePath, defaultPayload)
	requireStatus(t, defaultResponse, http.StatusCreated)
	var defaultData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
	if err := json.Unmarshal(defaultResponse.Body.Bytes(), &defaultData); err != nil { t.Fatal(err) }
	var expectedDefaults map[string]any
	if err := json.Unmarshal([]byte({{quote (defaultsJSON .Resource)}}), &expectedDefaults); err != nil { t.Fatal(err) }
	for name, want := range expectedDefaults {
		if got := defaultData.Data[name]; !reflect.DeepEqual(got, want) { t.Fatalf("default %s = %#v, want %#v", name, got, want) }
	}
	defaultID, ok := defaultData.Data["id"].(float64); if !ok { t.Fatalf("default id = %#v", defaultData.Data["id"]) }
	requireStatus(t, performJSON(t, e, http.MethodDelete, basePath+"/"+fmtID(int64(defaultID)), nil), http.StatusNoContent)
{{end}}
{{range defaultZeroFields .Resource}}	{
		// Explicit zero create values override database defaults.
		overridePayload := map[string]any{
{{range $.Resource.Fields}}{{if .Required}}			{{quote .Name}}: {{sampleGo .}},
{{end}}{{end}}			{{quote .Name}}: {{zeroGo .}},
		}
		overrideResponse := performJSON(t, e, http.MethodPost, basePath, overridePayload)
		requireStatus(t, overrideResponse, http.StatusCreated)
		var overrideData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
		if err := json.Unmarshal(overrideResponse.Body.Bytes(), &overrideData); err != nil { t.Fatal(err) }
		var want any; if err := json.Unmarshal([]byte({{quote (zeroExpectedJSON .)}}), &want); err != nil { t.Fatal(err) }
		if got := overrideData.Data[{{quote .Name}}]; !reflect.DeepEqual(got, want) { t.Fatalf("explicit zero {{.Name}} = %#v, want %#v", got, want) }
		overrideID := int64(overrideData.Data["id"].(float64))
		requireStatus(t, performJSON(t, e, http.MethodDelete, basePath+"/"+fmtID(overrideID), nil), http.StatusNoContent)
	}
{{end}}{{range nullableDefaultFields .Resource}}	{
		// Explicit null create values override non-null database defaults.
		nullDefaultPayload := map[string]any{
{{range $.Resource.Fields}}{{if .Required}}			{{quote .Name}}: {{sampleGo .}},
{{end}}{{end}}			{{quote .Name}}: nil,
		}
		nullDefaultResponse := performJSON(t, e, http.MethodPost, basePath, nullDefaultPayload)
		requireStatus(t, nullDefaultResponse, http.StatusCreated)
		var nullDefaultData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
		if err := json.Unmarshal(nullDefaultResponse.Body.Bytes(), &nullDefaultData); err != nil { t.Fatal(err) }
		if got := nullDefaultData.Data[{{quote .Name}}]; got != nil { t.Fatalf("explicit null {{.Name}} = %#v", got) }
		secondNullResponse := performJSON(t, e, http.MethodPost, basePath, nullDefaultPayload)
		requireStatus(t, secondNullResponse, http.StatusCreated)
		var secondNullData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
		if err := json.Unmarshal(secondNullResponse.Body.Bytes(), &secondNullData); err != nil { t.Fatal(err) }
		if got := secondNullData.Data[{{quote .Name}}]; got != nil { t.Fatalf("second explicit null {{.Name}} = %#v", got) }
		nullDefaultID := int64(nullDefaultData.Data["id"].(float64))
		secondNullID := int64(secondNullData.Data["id"].(float64))
		requireStatus(t, performJSON(t, e, http.MethodDelete, basePath+"/"+fmtID(nullDefaultID), nil), http.StatusNoContent)
		requireStatus(t, performJSON(t, e, http.MethodDelete, basePath+"/"+fmtID(secondNullID), nil), http.StatusNoContent)
	}
{{end}}
{{if optionalJSONFields .Resource}}	{
		// Omitted optional, non-null JSON fields use an empty object instead of SQL NULL.
		jsonPayload := map[string]any{
{{range .Resource.Fields}}{{if .Required}}			{{quote .Name}}: {{sampleGo .}},
{{end}}{{end}}		}
		jsonResponse := performJSON(t, e, http.MethodPost, basePath, jsonPayload)
		requireStatus(t, jsonResponse, http.StatusCreated)
		var jsonData struct { Data map[string]any ` + "`json:\"data\"`" + ` }
		if err := json.Unmarshal(jsonResponse.Body.Bytes(), &jsonData); err != nil { t.Fatal(err) }
{{range optionalJSONFields .Resource}}		if value, ok := jsonData.Data[{{quote .Name}}].(map[string]any); !ok || len(value) != 0 { t.Fatalf("omitted JSON {{.Name}} = %#v", jsonData.Data[{{quote .Name}}]) }
{{end}}		jsonID := int64(jsonData.Data["id"].(float64))
		requireStatus(t, performJSON(t, e, http.MethodDelete, basePath+"/"+fmtID(jsonID), nil), http.StatusNoContent)
	}
{{end}}

	// internal error redaction never exposes SQL, the SQLite DSN, or paths.
	sqlDB, err := db.DB(); if err != nil { t.Fatal(err) }
	if err := sqlDB.Close(); err != nil { t.Fatal(err) }
	internal := performJSON(t, e, http.MethodGet, basePath, nil)
	requireStatus(t, internal, http.StatusInternalServerError)
	lowerBody := strings.ToLower(internal.Body.String())
	for _, secret := range []string{"select ", "sqlite", ":memory:", "/internal/"} {
		if strings.Contains(lowerBody, secret) { t.Fatalf("internal error leaked %q: %s", secret, internal.Body.String()) }
	}
}

func performJSON(t *testing.T, e *echo.Echo, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Reader
	if payload == nil { body = bytes.NewReader(nil) } else { data, err := json.Marshal(payload); if err != nil { t.Fatal(err) }; body = bytes.NewReader(data) }
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	return response
}

func requireStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want { t.Fatalf("status = %d, want %d, body = %s", response.Code, want, response.Body.String()) }
}

func requireTotal(t *testing.T, response *httptest.ResponseRecorder, want int64) {
	t.Helper()
	var envelope struct { Data struct { Total int64 ` + "`json:\"total\"`" + ` } ` + "`json:\"data\"`" + ` }
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil { t.Fatal(err) }
	if envelope.Data.Total != want { t.Fatalf("total = %d, want %d, body = %s", envelope.Data.Total, want, response.Body.String()) }
}

func fmtID(id int64) string { return strconv.FormatInt(id, 10) }
`

const registerTemplate = `// Code generated by gobackend; DO NOT EDIT.

package generated

import (
	"github.com/labstack/echo/v5"
	"gorm.io/gorm"
{{range .Resources}}	"{{$.Module}}/internal/resources/{{.Package}}"
{{end}})

func Register(group *echo.Group, db *gorm.DB) {
{{range .Resources}}	{{.Package}}.Register(group, db)
{{end}}}
`

const gormSchemaTemplate = `// Code generated by gobackend; DO NOT EDIT.

package main

import (
	"fmt"
	"os"

	"ariga.io/atlas-provider-gorm/gormschema"
{{range .Resources}}	"{{$.Module}}/internal/resources/{{.Package}}"
{{end}})

func main() {
	models := []any{ {{range .Resources}}&{{.Package}}.{{.Name}}{},{{end}} }
	statements, err := gormschema.New("postgres").Load(models...)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
	fmt.Print(statements)
}
`

const openAPIEmbedTemplate = `// Code generated by gobackend; DO NOT EDIT.

package openapi

import _ "embed"

//go:embed openapi_gen.json
var Spec []byte
`
