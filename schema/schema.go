// Package schema provides JSON schema generation from Go types using reflection.
// It supports struct tags for customization: json, description and enum.
package schema

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// Schema represents a JSON schema definition. It is recursive, so nested
// structs and slices of structs keep their full structure.
type Schema struct {
	// Type is the JSON type: object, array, string, integer, number or boolean.
	Type string

	// Nullable marks the value as also accepting null. It is serialized as
	// a type union, e.g. "type": ["string", "null"].
	Nullable bool

	// Description explains the value to the model.
	Description string

	// Properties holds the fields of an object schema.
	Properties map[string]*Schema

	// Required lists the property names that must be present.
	Required []string

	// Items is the element schema of an array schema.
	Items *Schema

	// Enum restricts the value to a fixed set.
	Enum []any

	// Format is a string format hint such as "date-time".
	Format string

	// AdditionalProperties, when set, controls whether unknown object keys are allowed.
	AdditionalProperties *bool
}

// schemaJSON is the decoding shape of Schema. The type field stays raw
// because it can be either a string or an array.
type schemaJSON struct {
	Type                 json.RawMessage    `json:"type,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Format               string             `json:"format,omitempty"`
	AdditionalProperties *bool              `json:"additionalProperties,omitempty"`
}

// MarshalJSON encodes the schema, emitting a type union for nullable values.
// Object schemas always carry "properties" and "required", even when empty,
// because strict-mode validators reject objects without them.
func (s *Schema) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	switch {
	case s.Type != "" && s.Nullable:
		out["type"] = []string{s.Type, "null"}
	case s.Type != "":
		out["type"] = s.Type
	}
	if s.Type == "object" {
		out["properties"] = nonNilMap(s.Properties)
		out["required"] = nonNilSlice(s.Required)
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if s.Items != nil {
		out["items"] = s.Items
	}
	if len(s.Enum) > 0 {
		out["enum"] = s.Enum
	}
	if s.Format != "" {
		out["format"] = s.Format
	}
	if s.AdditionalProperties != nil {
		out["additionalProperties"] = *s.AdditionalProperties
	}
	return json.Marshal(out)
}

func nonNilMap(m map[string]*Schema) map[string]*Schema {
	if m == nil {
		return map[string]*Schema{}
	}
	return m
}

func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// UnmarshalJSON decodes a schema, accepting both "type": "T" and "type": ["T", "null"].
func (s *Schema) UnmarshalJSON(data []byte) error {
	var in schemaJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	*s = Schema{
		Description:          in.Description,
		Properties:           in.Properties,
		Required:             in.Required,
		Items:                in.Items,
		Enum:                 in.Enum,
		Format:               in.Format,
		AdditionalProperties: in.AdditionalProperties,
	}
	return s.unmarshalType(in.Type)
}

func (s *Schema) unmarshalType(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &s.Type); err == nil {
		return nil
	}
	var types []string
	if err := json.Unmarshal(raw, &types); err != nil {
		return fmt.Errorf("invalid schema type %s: %w", raw, err)
	}
	for _, t := range types {
		if t == "null" {
			s.Nullable = true
			continue
		}
		s.Type = t
	}
	return nil
}

// Strict returns a deep copy of the schema that satisfies OpenAI's strict
// structured-output rules: every object disallows additional properties and
// lists all of its properties as required. Properties that were optional
// become nullable instead, so the model can still omit a value by sending null.
func (s *Schema) Strict() *Schema {
	if s == nil {
		return nil
	}
	out := *s
	out.Required = nil
	out.Enum = slices.Clone(s.Enum)
	if out.Nullable && len(out.Enum) > 0 {
		out.Enum = append(out.Enum, nil)
	}
	out.Items = s.Items.Strict()

	if s.Type == "object" {
		closed := false
		out.AdditionalProperties = &closed
		out.Properties = make(map[string]*Schema, len(s.Properties))
		out.Required = slices.Sorted(maps.Keys(s.Properties))
		for name, prop := range s.Properties {
			strictProp := prop.Strict()
			if !slices.Contains(s.Required, name) {
				strictProp.Nullable = true
				if len(strictProp.Enum) > 0 && !slices.Contains(strictProp.Enum, nil) {
					strictProp.Enum = append(strictProp.Enum, nil)
				}
			}
			out.Properties[name] = strictProp
		}
	}
	return &out
}
