package generate

import (
	"fmt"
	"strings"

	"github.com/alphayan/go-backend-kit/internal/spec"
	"github.com/pb33f/libopenapi"
)

func buildOpenAPI(resources []spec.Resource, opts ProjectOptions) (map[string]any, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if err := validateOpenAPIComponentNames(resources, opts); err != nil {
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
	if opts.HasSession() {
		schemas["Login"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"email", "password"}, "properties": map[string]any{
			"email":    map[string]any{"type": "string", "format": "email", "maxLength": 320},
			"password": map[string]any{"type": "string", "format": "password", "maxLength": 1024},
		}}
		schemas["PasswordChange"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"current_password", "new_password"}, "properties": map[string]any{
			"current_password": map[string]any{"type": "string", "format": "password", "maxLength": 1024},
			"new_password":     map[string]any{"type": "string", "format": "password", "minLength": 12, "maxLength": 1024},
		}}
		schemas["Session"] = map[string]any{"type": "object", "required": []string{"id", "email", "role"}, "properties": map[string]any{
			"id": map[string]any{"type": "integer", "format": "int64"}, "email": map[string]any{"type": "string", "format": "email"}, "role": map[string]any{"type": "string", "enum": []string{"admin", "viewer"}},
		}}
		paths["/auth/login"] = map[string]any{"post": authOperation("Log in", ref("Login"), 200, dataSchema(ref("Session")), false, []string{"400", "401", "429", "500"})}
		paths["/auth/logout"] = map[string]any{"post": authOperation("Log out", nil, 204, nil, false, []string{"429", "500"})}
		paths["/auth/me"] = map[string]any{"get": authOperation("Current session user", nil, 200, dataSchema(ref("Session")), true, []string{"401", "500"})}
		paths["/auth/password"] = map[string]any{"post": authOperation("Change password", ref("PasswordChange"), 204, nil, true, []string{"400", "401", "429", "500"})}
		addAdminOpenAPI(paths, schemas)
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
			"get":  operation(tag, "List "+resource.Name, nil, 200, pageSchema(resource.Name), listParameters(resource), opts),
			"post": operation(tag, "Create "+resource.Name, ref("Create"+resource.Name+"Input"), 201, dataSchema(ref(resource.Name)), nil, opts),
		}
		paths[member] = map[string]any{
			"get":    operation(tag, "Get "+resource.Name, nil, 200, dataSchema(ref(resource.Name)), idParameters(), opts),
			"patch":  operation(tag, "Update "+resource.Name, ref("Update"+resource.Name+"Input"), 200, dataSchema(ref(resource.Name)), idParameters(), opts),
			"delete": operation(tag, "Delete "+resource.Name, nil, 204, nil, idParameters(), opts),
		}
	}
	components := map[string]any{"schemas": schemas}
	if opts.HasJWT() {
		components["securitySchemes"] = map[string]any{
			"bearerAuth": map[string]any{
				"type":         "http",
				"scheme":       "bearer",
				"bearerFormat": "JWT",
			},
		}
	} else if opts.HasSession() {
		cookieName := "session"
		if opts.IsProduction() {
			cookieName = "__Host-session"
		}
		components["securitySchemes"] = map[string]any{
			"sessionCookie": map[string]any{"type": "apiKey", "in": "cookie", "name": cookieName, "description": "Cookie name follows AUTH_SESSION_SECURE: __Host-session when true, session when false."},
		}
	}
	return map[string]any{
		"openapi":        "3.1.0",
		"x-generated-by": "gobackend",
		"info":           map[string]any{"title": "Generated Backend API", "version": "0.1.0"},
		"servers":        []any{map[string]any{"url": "/"}},
		"paths":          paths,
		"components":     components,
	}, nil
}

func validateOpenAPIComponentNames(resources []spec.Resource, opts ProjectOptions) error {
	owners := map[string]string{"Error": "built-in error response"}
	if opts.HasSession() {
		owners["Login"] = "session login input"
		owners["PasswordChange"] = "session password change input"
		owners["Session"] = "session user response"
		for _, name := range []string{"AuthUser", "AuthCreateUser", "AuthUpdateUser", "AuthSessionInfo", "AuthAuditLog"} {
			owners[name] = "session administration"
		}
	}
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

func operation(tag, summary string, body map[string]any, status int, responseSchema map[string]any, parameters []any, opts ProjectOptions) map[string]any {
	value := map[string]any{"tags": []string{tag}, "summary": summary, "responses": map[string]any{}}
	if len(parameters) > 0 {
		value["parameters"] = parameters
	}
	if body != nil {
		value["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": body}}}
	}
	if opts.HasJWT() {
		value["security"] = []any{map[string]any{"bearerAuth": []any{}}}
	} else if opts.HasSession() {
		value["security"] = []any{map[string]any{"sessionCookie": []any{}}}
	}
	responses := value["responses"].(map[string]any)
	response := map[string]any{"description": fmt.Sprintf("HTTP %d", status)}
	if responseSchema != nil {
		response["content"] = map[string]any{"application/json": map[string]any{"schema": responseSchema}}
	}
	responses[fmt.Sprint(status)] = response
	codes := []string{"400", "404", "409", "422", "500"}
	if opts.HasJWT() {
		codes = append([]string{"401"}, codes...)
	} else if opts.HasSession() {
		codes = append([]string{"401", "403", "429"}, codes...)
	}
	for _, code := range codes {
		responses[code] = map[string]any{"description": "Error", "content": map[string]any{"application/json": map[string]any{"schema": ref("Error")}}}
	}
	return value
}

func authOperation(summary string, body map[string]any, status int, responseSchema map[string]any, protected bool, errorCodes []string) map[string]any {
	value := map[string]any{"tags": []string{"Authentication"}, "summary": summary, "responses": map[string]any{}}
	if body != nil {
		value["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": body}}}
	}
	if protected {
		value["security"] = []any{map[string]any{"sessionCookie": []any{}}}
	}
	responses := value["responses"].(map[string]any)
	response := map[string]any{"description": fmt.Sprintf("HTTP %d", status)}
	if responseSchema != nil {
		response["content"] = map[string]any{"application/json": map[string]any{"schema": responseSchema}}
	}
	responses[fmt.Sprint(status)] = response
	for _, code := range errorCodes {
		responses[code] = map[string]any{"description": "Error", "content": map[string]any{"application/json": map[string]any{"schema": ref("Error")}}}
	}
	return value
}

func addAdminOpenAPI(paths, schemas map[string]any) {
	id := map[string]any{"type": "integer", "format": "int64", "minimum": 1}
	str := map[string]any{"type": "string"}
	timestamp := map[string]any{"type": "string", "format": "date-time"}
	role := map[string]any{"type": "string", "enum": []string{"admin", "viewer"}}
	email := map[string]any{"type": "string", "format": "email", "maxLength": 320}
	schemas["AuthUser"] = map[string]any{"type": "object", "required": []string{"id", "email", "role", "created_at", "updated_at"}, "properties": map[string]any{
		"id": id, "email": email, "role": role, "disabled_at": timestamp, "created_at": timestamp, "updated_at": timestamp,
	}}
	schemas["AuthCreateUser"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"email", "password", "role"}, "properties": map[string]any{
		"email": email, "role": role, "password": map[string]any{"type": "string", "format": "password", "writeOnly": true, "minLength": 12, "maxLength": 1024, "description": "Server validates 12 to 1024 UTF-8 bytes."},
	}}
	schemas["AuthUpdateUser"] = map[string]any{"type": "object", "additionalProperties": false, "minProperties": 1, "description": "Cannot change your own role or status. Revokes all target sessions.", "properties": map[string]any{
		"role": role, "disabled": map[string]any{"type": "boolean"},
	}}
	schemas["AuthSessionInfo"] = map[string]any{"type": "object", "required": []string{"id", "created_at", "expires_at", "last_seen_at", "user_agent", "ip"}, "properties": map[string]any{
		"id": id, "created_at": timestamp, "expires_at": timestamp, "last_seen_at": timestamp, "user_agent": str, "ip": str,
	}}
	schemas["AuthAuditLog"] = map[string]any{"type": "object", "required": []string{"id", "actor_id", "action", "resource", "resource_id", "outcome", "request_id", "created_at"}, "properties": map[string]any{
		"id": id, "actor_id": map[string]any{"type": []string{"integer", "null"}, "format": "int64"}, "action": str, "resource": str, "resource_id": str, "outcome": str, "request_id": str, "created_at": timestamp,
	}}
	pageParams := []any{queryParameter("page", "integer"), queryParameter("page_size", "integer")}
	adminOperation := func(summary string, body map[string]any, status int, response map[string]any, params []any) map[string]any {
		op := authOperation(summary, body, status, response, true, []string{"400", "401", "403", "404", "409", "429", "500"})
		op["tags"] = []string{"Administration"}
		op["description"] = "Requires an active admin session. Responses must not be cached."
		if len(params) > 0 {
			op["parameters"] = params
		}
		return op
	}
	paths["/auth/users"] = map[string]any{
		"get":  adminOperation("List users (exact email filter)", nil, 200, pageSchema("AuthUser"), append(append([]any{}, pageParams...), queryParameter("email", "string"))),
		"post": adminOperation("Create user", ref("AuthCreateUser"), 201, dataSchema(ref("AuthUser")), nil),
	}
	paths["/auth/users/{id}"] = map[string]any{"patch": adminOperation("Update user access", ref("AuthUpdateUser"), 200, dataSchema(ref("AuthUser")), idParameters())}
	paths["/auth/users/{id}/sessions"] = map[string]any{
		"get":    adminOperation("List active user sessions", nil, 200, pageSchema("AuthSessionInfo"), append(idParameters(), pageParams...)),
		"delete": adminOperation("Revoke all user sessions", nil, 204, nil, idParameters()),
	}
	paths["/auth/users/{id}/sessions/{session_id}"] = map[string]any{"delete": adminOperation("Revoke one user session", nil, 204, nil,
		append(idParameters(), map[string]any{"name": "session_id", "in": "path", "required": true, "schema": id}))}
	filters := append([]any{}, pageParams...)
	for _, field := range []string{"action", "resource", "outcome"} {
		filters = append(filters, queryParameter(field, "string"))
	}
	filters = append(filters, queryParameter("actor_id", "integer"))
	for _, field := range []string{"from", "to"} {
		filters = append(filters, map[string]any{"name": field, "in": "query", "required": false, "schema": timestamp, "description": "RFC3339 timestamp; from inclusive, to exclusive."})
	}
	paths["/auth/audit-logs"] = map[string]any{"get": adminOperation("List audit logs newest first", nil, 200, pageSchema("AuthAuditLog"), filters)}
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
		parts[i] = "LOWER(" + sqlColumn(column) + ") LIKE ? ESCAPE '\\'"
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
