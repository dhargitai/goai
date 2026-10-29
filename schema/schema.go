// Package schema provides JSON schema generation from Go types using reflection.
// It supports struct tags for customization including json, validate, description, and enum tags.
package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
)

// Schema represents a JSON schema definition.
type Schema struct {
	Type        string              `json:"type,omitempty"`
	Properties  map[string]Property `json:"properties,omitempty"`
	Required    []string            `json:"required,omitempty"`
	Description string              `json:"description,omitempty"`
	Items       *Schema             `json:"items,omitempty"`
	Enum        []any               `json:"enum,omitempty"`
}

// Property represents a property within a JSON schema.
type Property struct {
	Type        string    `json:"type,omitempty"`
	Description string    `json:"description,omitempty"`
	Enum        []any     `json:"enum,omitempty"`
	Items       *Property `json:"items,omitempty"`
	Format      string    `json:"format,omitempty"`
}

// Generate creates a JSON schema from a Go type using reflection.
// It supports struct tags:
//   - `json:"field_name"` - JSON field name
//   - `validate:"required"` - Required field (automatically added for all non-pointer fields)
//   - `description:"field description"` - Field description
//   - `enum:"value1,value2"` - Enum values
//
// Example:
//
//	type User struct {
//	    Name  string `json:"name" description:"User's full name"`
//	    Email string `json:"email" description:"User's email address"`
//	    Role  string `json:"role" enum:"admin,user,guest"`
//	}
//
//	schema, err := schema.Generate(reflect.TypeOf(User{}))
func Generate(t reflect.Type) (*Schema, error) {
	if t == nil {
		return nil, fmt.Errorf("type cannot be nil")
	}

	// Handle pointer types
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	// For primitive types, handle directly
	if isPrimitiveKind(t.Kind()) {
		return &Schema{
			Type: goKindToJSONType(t.Kind()),
		}, nil
	}

	// For slices, handle directly
	if t.Kind() == reflect.Slice {
		elemType := t.Elem()
		var itemSchema *Schema
		var err error

		if isPrimitiveKind(elemType.Kind()) {
			itemSchema = &Schema{
				Type: goKindToJSONType(elemType.Kind()),
			}
		} else {
			itemSchema, err = Generate(elemType)
			if err != nil {
				return nil, fmt.Errorf("failed to generate schema for slice element: %w", err)
			}
		}

		return &Schema{
			Type:  "array",
			Items: itemSchema,
		}, nil
	}

	// For structs, use jsonschema library
	value := reflect.New(t).Elem().Interface()

	// Use jsonschema library for robust schema generation
	reflector := &jsonschema.Reflector{
		AllowAdditionalProperties: false,
		ExpandedStruct:            true, // Inline structs instead of using $ref
	}

	jsSchema := reflector.Reflect(value)

	// Convert to our Schema type
	schema, err := convertJSONSchema(jsSchema, reflector)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	return schema, nil
}

// isPrimitiveKind returns true if the kind is a Go primitive type.
func isPrimitiveKind(k reflect.Kind) bool {
	switch k {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	default:
		return false
	}
}

// goKindToJSONType converts a Go reflect.Kind to a JSON schema type.
func goKindToJSONType(k reflect.Kind) string {
	switch k {
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.String:
		return "string"
	default:
		return ""
	}
}

// convertJSONSchema converts jsonschema.Schema to our Schema type.
func convertJSONSchema(js *jsonschema.Schema, reflector *jsonschema.Reflector) (*Schema, error) {
	schema := &Schema{
		Type:        string(js.Type),
		Description: js.Description,
		Required:    js.Required,
	}

	// Handle properties
	if js.Properties != nil {
		schema.Properties = make(map[string]Property)

		// Iterate through the ordered map
		for pair := js.Properties.Oldest(); pair != nil; pair = pair.Next() {
			key := pair.Key
			value := pair.Value

			prop, err := convertProperty(value, js, reflector)
			if err != nil {
				return nil, fmt.Errorf("failed to convert property %s: %w", key, err)
			}

			schema.Properties[key] = prop
		}
	}

	// Handle items for arrays
	if js.Items != nil {
		itemSchema, err := convertJSONSchema(js.Items, reflector)
		if err != nil {
			return nil, err
		}
		schema.Items = itemSchema
	}

	// Handle enum
	if js.Enum != nil && len(js.Enum) > 0 {
		schema.Enum = js.Enum
	}

	return schema, nil
}

// convertProperty converts a jsonschema property to our Property type.
func convertProperty(value *jsonschema.Schema, parent *jsonschema.Schema, reflector *jsonschema.Reflector) (Property, error) {
	prop := Property{
		Type:        string(value.Type),
		Description: value.Description,
	}

	// Handle $ref for nested objects
	if value.Ref != "" {
		// The type is empty when using $ref, so we need to set it to "object"
		prop.Type = "object"

		// Try to resolve the ref from parent.$defs
		if parent.Definitions != nil {
			refName := strings.TrimPrefix(value.Ref, "#/$defs/")
			if def := parent.Definitions[refName]; def != nil {
				// For nested objects, we just mark it as "object"
				// The actual nested properties are in the $ref which is expanded by the library
				prop.Type = "object"
			}
		}
	}

	// Handle enum values
	if value.Enum != nil && len(value.Enum) > 0 {
		prop.Enum = value.Enum
	}

	// Handle format (for strings like email, uri, etc.)
	if value.Format != "" {
		prop.Format = value.Format
	}

	// Handle arrays
	if value.Items != nil {
		itemProp := Property{
			Type:        string(value.Items.Type),
			Description: value.Items.Description,
		}
		prop.Items = &itemProp
	}

	// Handle nested properties for objects
	if value.Properties != nil && value.Properties.Len() > 0 {
		// This is an inline object, ensure type is set
		if prop.Type == "" {
			prop.Type = "object"
		}
	}

	return prop, nil
}

// GenerateFromValue creates a JSON schema from a Go value.
// This is a convenience function that extracts the type from the value.
func GenerateFromValue(v any) (*Schema, error) {
	if v == nil {
		return nil, fmt.Errorf("value cannot be nil")
	}

	t := reflect.TypeOf(v)
	return Generate(t)
}

// MarshalJSON converts a Schema to JSON bytes.
func (s *Schema) MarshalJSON() ([]byte, error) {
	type Alias Schema
	return json.Marshal((*Alias)(s))
}

// UnmarshalJSON populates a Schema from JSON bytes.
func (s *Schema) UnmarshalJSON(data []byte) error {
	type Alias Schema
	return json.Unmarshal(data, (*Alias)(s))
}

// parseEnumTag parses comma-separated enum values from a struct tag.
func parseEnumTag(tag string) []any {
	if tag == "" {
		return nil
	}

	parts := strings.Split(tag, ",")
	enum := make([]any, len(parts))
	for i, part := range parts {
		enum[i] = strings.TrimSpace(part)
	}
	return enum
}
