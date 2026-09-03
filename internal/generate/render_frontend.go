package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alphayan/go-backend-kit/internal/spec"
)

func renderFrontendResources(resources []spec.Resource) (map[string][]byte, error) {
	files := make(map[string][]byte, len(resources)+1)
	for _, resource := range resources {
		output, err := renderFrontendResource(resource)
		if err != nil {
			return nil, fmt.Errorf("render frontend resource %s: %w", resource.Name, err)
		}
		files["web/src/generated/"+resource.Package+"_gen.ts"] = output
	}
	files["web/src/generated/registry_gen.ts"] = renderFrontendRegistry(resources)
	return files, nil
}

func renderFrontendResource(resource spec.Resource) ([]byte, error) {
	var output bytes.Buffer
	fmt.Fprintf(&output, "// %s\n\n", generatedMarker)
	output.WriteString("import { z } from \"zod\"\n")
	output.WriteString("import { dateTimeSchema, decimalSchema, jsonSchema, numberSchema } from \"@/lib/forms\"\n")
	output.WriteString("import type { FieldDefinition, ResourceDefinition } from \"@/lib/resource\"\n\n")

	identifier := resource.Package
	fmt.Fprintf(&output, "export const %sCreateSchema = z.object({\n", identifier)
	for _, field := range resource.Fields {
		fmt.Fprintf(&output, "  %s: %s,\n", field.Name, frontendSchema(field))
	}
	output.WriteString("})\n")
	fmt.Fprintf(&output, "export const %sUpdateSchema = %sCreateSchema.partial()\n\n", identifier, identifier)

	fmt.Fprintf(&output, "const fields = %s satisfies FieldDefinition[]\n\n", mustFrontendJSON(frontendFields(resource.Fields)))
	fmt.Fprintf(&output, "export const %sResource = {\n", identifier)
	fmt.Fprintf(&output, "  key: %s,\n", mustFrontendJSON(resource.Package))
	fmt.Fprintf(&output, "  name: %s,\n", mustFrontendJSON(resource.Name))
	fmt.Fprintf(&output, "  route: %s,\n", mustFrontendJSON(resource.Route))
	fmt.Fprintf(&output, "  apiPath: %s,\n", mustFrontendJSON("/api/v1"+resource.Route))
	output.WriteString("  fields,\n")
	fmt.Fprintf(&output, "  columns: %s,\n", mustFrontendJSON(frontendColumns(resource)))
	fmt.Fprintf(&output, "  createSchema: %sCreateSchema,\n", identifier)
	fmt.Fprintf(&output, "  updateSchema: %sUpdateSchema,\n", identifier)
	output.WriteString("} satisfies ResourceDefinition\n")
	return output.Bytes(), nil
}

func renderFrontendRegistry(resources []spec.Resource) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "// %s\n\n", generatedMarker)
	output.WriteString("import type { ResourceDefinition } from \"@/lib/resource\"\n")
	for _, resource := range resources {
		fmt.Fprintf(&output, "import { %sResource } from \"./%s_gen\"\n", resource.Package, resource.Package)
	}
	output.WriteString("\nexport const resources: readonly ResourceDefinition[] = [\n")
	for _, resource := range resources {
		fmt.Fprintf(&output, "  %sResource,\n", resource.Package)
	}
	output.WriteString("]\n")
	return output.Bytes()
}

func frontendSchema(field spec.Field) string {
	var schema string
	switch field.Type {
	case spec.TypeString, spec.TypeText:
		if len(field.Enum) > 0 {
			schema = "z.enum(" + mustFrontendJSON(field.Enum) + ")"
		} else {
			schema = "z.string()"
			if field.MaxLength != nil {
				schema += fmt.Sprintf(".max(%d)", *field.MaxLength)
			}
		}
	case spec.TypeBool:
		schema = "z.boolean()"
	case spec.TypeInt32, spec.TypeInt64:
		schema = fmt.Sprintf("numberSchema(true, %s, %s)", frontendNumber(field.Min), frontendNumber(field.Max))
	case spec.TypeFloat64:
		schema = fmt.Sprintf("numberSchema(false, %s, %s)", frontendNumber(field.Min), frontendNumber(field.Max))
	case spec.TypeDecimal:
		schema = fmt.Sprintf("decimalSchema(%s, %s)", frontendNumber(field.Min), frontendNumber(field.Max))
	case spec.TypeTime:
		schema = "dateTimeSchema"
	case spec.TypeUUID:
		schema = "z.uuid()"
	case spec.TypeJSON:
		schema = "jsonSchema"
	default:
		panic("unsupported frontend field type")
	}
	if field.Nullable {
		schema += ".nullable()"
	}
	if !field.Required {
		schema += ".optional()"
	}
	return schema
}

func frontendNumber(value *spec.Number) string {
	if value == nil {
		return "undefined"
	}
	return mustFrontendJSON(value.String())
}

func frontendFields(fields []spec.Field) []map[string]any {
	result := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		enumValues := field.Enum
		if enumValues == nil {
			enumValues = []string{}
		}
		value := map[string]any{
			"name":       field.Name,
			"label":      frontendLabel(field.Name),
			"type":       field.Type,
			"required":   field.Required,
			"nullable":   field.Nullable,
			"hasDefault": field.HasDefault,
			"enumValues": enumValues,
			"filterable": field.Filterable,
			"sortable":   field.Sortable,
			"searchable": field.Searchable,
		}
		if field.HasDefault {
			value["defaultValue"] = field.Default
		}
		if field.Min != nil {
			value["min"] = field.Min.String()
		}
		if field.Max != nil {
			value["max"] = field.Max.String()
		}
		if field.MaxLength != nil {
			value["maxLength"] = *field.MaxLength
		}
		result = append(result, value)
	}
	return result
}

func frontendColumns(resource spec.Resource) []map[string]any {
	columns := []map[string]any{{"key": "id", "label": "ID", "kind": "id", "sortable": true}}
	for _, field := range resource.Fields {
		columns = append(columns, map[string]any{
			"key": field.Name, "label": frontendLabel(field.Name), "kind": field.Type, "sortable": field.Sortable,
		})
	}
	return append(columns,
		map[string]any{"key": "created_at", "label": "Created", "kind": "timestamp", "sortable": true},
		map[string]any{"key": "updated_at", "label": "Updated", "kind": "timestamp", "sortable": false},
	)
}

func frontendLabel(name string) string {
	parts := strings.Split(name, "_")
	for i, part := range parts {
		if part == "id" {
			parts[i] = "ID"
			continue
		}
		if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func mustFrontendJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}
