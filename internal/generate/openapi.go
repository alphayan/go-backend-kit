package generate

import (
	"fmt"
	"strings"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"github.com/pb33f/libopenapi"
)

func buildOpenAPI(resources []spec.Resource) (map[string]any, error) {
	if err := validateOpenAPIComponentNames(resources); err != nil {
		return nil, err
	}
	paths := map[string]any{}
	schemas := map[string]any{
		"Error": map[string]any{
			"type": "object", "required": []string{"error"},
			"properties": map[string]any{"error": map[string]any{"type": "object", "required": []string{"code", "message", "request_id"}, "properties": map[string]any{
				"code": map[string]any{"type": "string"}, "message": map[string]any{"type": "string"}, "details": map[string]any{}, "request_id": map[string]any{"type": "string"},
			}}},
		},
	}
	for _, resource := range resources {
		model, create, update := openAPISchemas(resource)
		schemas[resource.Name] = model
		schemas["Create"+resource.Name+"Input"] = create
		schemas["Update"+resource.Name+"Input"] = update
		tag := resource.Name
		collection := "/api/v1" + resource.Route
		member := collection + "/{id}"
		paths[collection] = map[string]any{
			"get":  operation(tag, "List "+resource.Name, nil, 200, pageSchema(resource.Name), listParameters(resource)),
			"post": operation(tag, "Create "+resource.Name, ref("Create"+resource.Name+"Input"), 201, dataSchema(ref(resource.Name)), nil),
		}
		paths[member] = map[string]any{
			"get":    operation(tag, "Get "+resource.Name, nil, 200, dataSchema(ref(resource.Name)), idParameters()),
			"patch":  operation(tag, "Update "+resource.Name, ref("Update"+resource.Name+"Input"), 200, dataSchema(ref(resource.Name)), idParameters()),
			"delete": operation(tag, "Delete "+resource.Name, nil, 204, nil, idParameters()),
		}
	}
	return map[string]any{
		"openapi":        "3.1.0",
		"x-generated-by": "gobackend",
		"info":           map[string]any{"title": "Generated Backend API", "version": "0.1.0"},
		"servers":        []any{map[string]any{"url": "/"}},
		"paths":          paths,
		"components":     map[string]any{"schemas": schemas},
	}, nil
}

func validateOpenAPIComponentNames(resources []spec.Resource) error {
	owners := map[string]string{"Error": "built-in error response"}
	claim := func(name, owner string) error {
		if existing, exists := owners[name]; exists {
			return fmt.Errorf("OpenAPI component schema %q is claimed by both %s and %s", name, existing, owner)
		}
		owners[name] = owner
		return nil
	}
	for _, resource := range resources {
		claims := []struct {
			name  string
			owner string
		}{
			{resource.Name, fmt.Sprintf("resource %q model", resource.Name)},
			{"Create" + resource.Name + "Input", fmt.Sprintf("resource %q create input", resource.Name)},
			{"Update" + resource.Name + "Input", fmt.Sprintf("resource %q update input", resource.Name)},
		}
		for _, candidate := range claims {
			if err := claim(candidate.name, candidate.owner); err != nil {
				return err
			}
		}
	}
	return nil
}

func openAPISchemas(resource spec.Resource) (map[string]any, map[string]any, map[string]any) {
	modelProperties := map[string]any{
		"id":         map[string]any{"type": "integer", "format": "int64", "readOnly": true},
		"created_at": map[string]any{"type": "string", "format": "date-time", "readOnly": true},
		"updated_at": map[string]any{"type": "string", "format": "date-time", "readOnly": true},
	}
	createProperties, updateProperties := map[string]any{}, map[string]any{}
	modelRequired := []string{"id", "created_at", "updated_at"}
	var createRequired []string
	for _, field := range resource.Fields {
		schema := fieldSchema(field)
		modelProperties[field.Name] = schema
		createProperties[field.Name] = schema
		updateProperties[field.Name] = schema
		if !field.Nullable {
			modelRequired = append(modelRequired, field.Name)
		}
		if field.Required {
			createRequired = append(createRequired, field.Name)
		}
	}
	model := map[string]any{"type": "object", "properties": modelProperties, "required": modelRequired}
	create := map[string]any{"type": "object", "properties": createProperties, "additionalProperties": false}
	if len(createRequired) > 0 {
		create["required"] = createRequired
	}
	update := map[string]any{"type": "object", "properties": updateProperties, "additionalProperties": false, "minProperties": 1}
	return model, create, update
}

func fieldSchema(field spec.Field) map[string]any {
	value := map[string]any{}
	switch field.Type {
	case spec.TypeString, spec.TypeText:
		value["type"] = "string"
	case spec.TypeBool:
		value["type"] = "boolean"
	case spec.TypeInt32:
		value["type"], value["format"] = "integer", "int32"
	case spec.TypeInt64:
		value["type"], value["format"] = "integer", "int64"
	case spec.TypeFloat64:
		value["type"], value["format"] = "number", "double"
	case spec.TypeDecimal:
		value["type"], value["format"] = "string", "decimal"
		value["pattern"] = `^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`
	case spec.TypeTime:
		value["type"], value["format"] = "string", "date-time"
	case spec.TypeUUID:
		value["type"], value["format"] = "string", "uuid"
	case spec.TypeJSON:
		value = map[string]any{"not": map[string]any{"type": "null"}}
	}
	if field.MaxLength != nil {
		value["maxLength"] = *field.MaxLength
	}
	if len(field.Enum) > 0 {
		value["enum"] = field.Enum
	}
	if field.Min != nil {
		if field.Type == spec.TypeDecimal {
			value["x-minimum"] = *field.Min
		} else {
			value["minimum"] = *field.Min
		}
	}
	if field.Max != nil {
		if field.Type == spec.TypeDecimal {
			value["x-maximum"] = *field.Max
		} else {
			value["maximum"] = *field.Max
		}
	}
	if field.HasDefault {
		value["default"] = field.Default
	}
	if field.Nullable {
		value = map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}
	}
	return value
}

func operation(tag, summary string, body map[string]any, status int, responseSchema map[string]any, parameters []any) map[string]any {
	value := map[string]any{"tags": []string{tag}, "summary": summary, "responses": map[string]any{}}
	if len(parameters) > 0 {
		value["parameters"] = parameters
	}
	if body != nil {
		value["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": body}}}
	}
	responses := value["responses"].(map[string]any)
	response := map[string]any{"description": fmt.Sprintf("HTTP %d", status)}
	if responseSchema != nil {
		response["content"] = map[string]any{"application/json": map[string]any{"schema": responseSchema}}
	}
	responses[fmt.Sprint(status)] = response
	for _, code := range []string{"400", "404", "409", "422", "500"} {
		responses[code] = map[string]any{"description": "Error", "content": map[string]any{"application/json": map[string]any{"schema": ref("Error")}}}
	}
	return value
}

func ref(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }

func dataSchema(data map[string]any) map[string]any {
	return map[string]any{"type": "object", "required": []string{"data"}, "properties": map[string]any{"data": data}}
}

func pageSchema(name string) map[string]any {
	return dataSchema(map[string]any{"type": "object", "required": []string{"items", "page", "page_size", "total", "total_pages"}, "properties": map[string]any{
		"items": map[string]any{"type": "array", "items": ref(name)}, "page": map[string]any{"type": "integer"}, "page_size": map[string]any{"type": "integer"}, "total": map[string]any{"type": "integer", "format": "int64"}, "total_pages": map[string]any{"type": "integer"},
	}})
}

func idParameters() []any {
	return []any{map[string]any{"name": "id", "in": "path", "required": true, "schema": map[string]any{"type": "integer", "format": "int64", "minimum": 1}}}
}

func listParameters(resource spec.Resource) []any {
	parameters := []any{
		queryParameter("page", "integer"), queryParameter("page_size", "integer"),
		queryParameter("sort", "string"), queryParameter("q", "string"),
	}
	for _, field := range resource.Fields {
		if field.Filterable {
			parameter := queryParameter(field.Name, openAPIPrimitive(field.Type))
			parameters = append(parameters, parameter)
		}
	}
	return parameters
}

func queryParameter(name, typ string) map[string]any {
	return map[string]any{"name": name, "in": "query", "required": false, "schema": map[string]any{"type": typ}}
}

func openAPIPrimitive(fieldType spec.FieldType) string {
	switch fieldType {
	case spec.TypeBool:
		return "boolean"
	case spec.TypeInt32, spec.TypeInt64:
		return "integer"
	case spec.TypeFloat64:
		return "number"
	default:
		return "string"
	}
}

func buildSearchSQL(resource spec.Resource) string {
	columns := searchColumns(resource)
	parts := make([]string, len(columns))
	for i, column := range columns {
		parts[i] = "LOWER(" + sqlColumn(column) + ") LIKE ?"
	}
	return strings.Join(parts, " OR ")
}

func sqlColumn(column string) string {
	return `"` + column + `"`
}

func validateOpenAPI(data []byte) error {
	document, err := libopenapi.NewDocument(data)
	if err != nil {
		return fmt.Errorf("parse generated OpenAPI 3.1: %w", err)
	}
	if _, err := document.BuildV3Model(); err != nil {
		return fmt.Errorf("validate generated OpenAPI 3.1: %w", err)
	}
	return nil
}
