package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	timeType       = reflect.TypeFor[time.Time]()
	rawMessageType = reflect.TypeFor[json.RawMessage]()
)

// Generate creates a JSON schema from a Go type using reflection.
// Nested structs, slices of structs and pointers are fully expanded.
//
// Supported struct tags:
//   - `json:"field_name"` - property name; "-" skips the field, and
//     omitempty/omitzero make it optional
//   - `description:"..."` - property description shown to the model
//   - `enum:"a,b,c"` - allowed values
//
// Non-pointer fields without omitempty/omitzero are required. Recursive types
// are not supported and return an error.
//
// Example:
//
//	type User struct {
//	    Name  string `json:"name" description:"User's full name"`
//	    Email string `json:"email" description:"User's email address"`
//	    Role  string `json:"role" enum:"admin,user,guest"`
//	}
//
//	schema, err := schema.Generate(reflect.TypeFor[User]())
func Generate(t reflect.Type) (*Schema, error) {
	if t == nil {
		return nil, fmt.Errorf("type cannot be nil")
	}
	g := generator{visiting: map[reflect.Type]bool{}}
	return g.schemaFor(t)
}

// GenerateFromValue creates a JSON schema from a Go value.
// This is a convenience function that extracts the type from the value.
func GenerateFromValue(v any) (*Schema, error) {
	if v == nil {
		return nil, fmt.Errorf("value cannot be nil")
	}
	return Generate(reflect.TypeOf(v))
}

type generator struct {
	// visiting holds the struct types on the current path, to detect recursion.
	visiting map[reflect.Type]bool
}

func (g generator) schemaFor(t reflect.Type) (*Schema, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t {
	case timeType:
		return &Schema{Type: "string", Format: "date-time"}, nil
	case rawMessageType:
		return &Schema{}, nil
	}

	switch t.Kind() {
	case reflect.Struct:
		return g.structSchema(t)
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return &Schema{Type: "string"}, nil // []byte marshals as base64
		}
		items, err := g.schemaFor(t.Elem())
		if err != nil {
			return nil, fmt.Errorf("array element: %w", err)
		}
		return &Schema{Type: "array", Items: items}, nil
	case reflect.Map:
		return &Schema{Type: "object"}, nil
	case reflect.Interface:
		return &Schema{}, nil
	}

	jsonType := primitiveJSONType(t.Kind())
	if jsonType == "" {
		return nil, fmt.Errorf("unsupported type %s", t)
	}
	return &Schema{Type: jsonType}, nil
}

func (g generator) structSchema(t reflect.Type) (*Schema, error) {
	if g.visiting[t] {
		return nil, fmt.Errorf("recursive type %s is not supported", t)
	}
	g.visiting[t] = true
	defer delete(g.visiting, t)

	s := &Schema{Type: "object", Properties: map[string]*Schema{}}
	if err := g.addFields(s, t); err != nil {
		return nil, err
	}
	return s, nil
}

// addFields adds the fields of struct type t to s, flattening embedded structs
// the same way encoding/json does.
func (g generator) addFields(s *Schema, t reflect.Type) error {
	for field := range t.Fields() {
		name, opts, skip := jsonFieldName(field)
		if skip {
			continue
		}

		if field.Anonymous && name == "" && derefKind(field.Type) == reflect.Struct {
			if err := g.addFields(s, derefType(field.Type)); err != nil {
				return err
			}
			continue
		}
		if name == "" {
			name = field.Name
		}

		prop, err := g.fieldSchema(field)
		if err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}
		s.Properties[name] = prop

		optional := field.Type.Kind() == reflect.Pointer ||
			strings.Contains(opts, "omitempty") || strings.Contains(opts, "omitzero")
		if !optional {
			s.Required = append(s.Required, name)
		}
	}
	return nil
}

func (g generator) fieldSchema(field reflect.StructField) (*Schema, error) {
	prop, err := g.schemaFor(field.Type)
	if err != nil {
		return nil, err
	}
	if desc := field.Tag.Get("description"); desc != "" {
		prop.Description = desc
	}
	if tag := field.Tag.Get("enum"); tag != "" {
		enum, err := parseEnum(tag, derefKind(field.Type))
		if err != nil {
			return nil, err
		}
		prop.Enum = enum
	}
	return prop, nil
}

// jsonFieldName returns the JSON name and options of a struct field, and
// whether encoding/json would skip it.
func jsonFieldName(field reflect.StructField) (name, opts string, skip bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", "", true
	}
	name, opts, _ = strings.Cut(tag, ",")
	if !field.IsExported() && (!field.Anonymous || name != "") {
		return "", "", true
	}
	return name, opts, false
}

// parseEnum parses comma-separated enum values, converting them to the field's kind.
func parseEnum(tag string, kind reflect.Kind) ([]any, error) {
	parts := strings.Split(tag, ",")
	enum := make([]any, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		value, err := parseEnumValue(part, kind)
		if err != nil {
			return nil, fmt.Errorf("invalid enum value %q: %w", part, err)
		}
		enum = append(enum, value)
	}
	return enum, nil
}

func parseEnumValue(v string, kind reflect.Kind) (any, error) {
	switch primitiveJSONType(kind) {
	case "integer":
		return strconv.ParseInt(v, 10, 64)
	case "number":
		return strconv.ParseFloat(v, 64)
	case "boolean":
		return strconv.ParseBool(v)
	default:
		return v, nil
	}
}

func primitiveJSONType(k reflect.Kind) string {
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

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func derefKind(t reflect.Type) reflect.Kind {
	return derefType(t).Kind()
}
