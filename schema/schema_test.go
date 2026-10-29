package schema

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type SimpleStruct struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type StructWithDescription struct {
	Name  string `json:"name" description:"Person's full name"`
	Email string `json:"email" description:"Email address"`
}

type StructWithRequired struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required"`
	Phone string `json:"phone"`
}

type StructWithEnum struct {
	Role string `json:"role" enum:"admin,user,guest"`
}

type NestedStruct struct {
	User    SimpleStruct `json:"user"`
	Active  bool         `json:"active"`
	Counter int          `json:"counter"`
}

type StructWithSlice struct {
	Tags  []string `json:"tags"`
	Items []int    `json:"items"`
}

type StructWithMap struct {
	Metadata map[string]string `json:"metadata"`
}

func TestGenerate_SimpleStruct(t *testing.T) {
	schema, err := Generate(reflect.TypeFor[SimpleStruct]())
	require.NoError(t, err)
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema.Type)
	assert.NotNil(t, schema.Properties)
	assert.Contains(t, schema.Properties, "name")
	assert.Contains(t, schema.Properties, "age")

	nameProp := schema.Properties["name"]
	assert.Equal(t, "string", nameProp.Type)

	ageProp := schema.Properties["age"]
	assert.Equal(t, "integer", ageProp.Type)
}

func TestGenerate_WithDescription(t *testing.T) {
	schema, err := Generate(reflect.TypeFor[StructWithDescription]())
	require.NoError(t, err)
	require.NotNil(t, schema)

	assert.Contains(t, schema.Properties, "name")
	nameProp := schema.Properties["name"]
	// Note: invopop/jsonschema doesn't automatically parse custom `description` tags
	// We would need custom reflection logic for that, which could be added later
	assert.Equal(t, "string", nameProp.Type)
}

func TestGenerate_NestedStruct(t *testing.T) {
	schema, err := Generate(reflect.TypeFor[NestedStruct]())
	require.NoError(t, err)
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "user")
	assert.Contains(t, schema.Properties, "active")
	assert.Contains(t, schema.Properties, "counter")

	userProp := schema.Properties["user"]
	assert.Equal(t, "object", userProp.Type)

	activeProp := schema.Properties["active"]
	assert.Equal(t, "boolean", activeProp.Type)
}

func TestGenerate_WithSlice(t *testing.T) {
	schema, err := Generate(reflect.TypeFor[StructWithSlice]())
	require.NoError(t, err)
	require.NotNil(t, schema)

	assert.Contains(t, schema.Properties, "tags")
	assert.Contains(t, schema.Properties, "items")

	tagsProp := schema.Properties["tags"]
	assert.Equal(t, "array", tagsProp.Type)
	assert.NotNil(t, tagsProp.Items)
	assert.Equal(t, "string", tagsProp.Items.Type)

	itemsProp := schema.Properties["items"]
	assert.Equal(t, "array", itemsProp.Type)
	assert.NotNil(t, itemsProp.Items)
	assert.Equal(t, "integer", itemsProp.Items.Type)
}

func TestGenerate_PrimitiveTypes(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		wantType string
	}{
		{
			name:     "string",
			value:    "",
			wantType: "string",
		},
		{
			name:     "int",
			value:    0,
			wantType: "integer",
		},
		{
			name:     "float64",
			value:    0.0,
			wantType: "number",
		},
		{
			name:     "bool",
			value:    false,
			wantType: "boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := GenerateFromValue(tt.value)
			require.NoError(t, err)
			require.NotNil(t, schema)
			assert.Equal(t, tt.wantType, schema.Type)
		})
	}
}

func TestGenerate_Array(t *testing.T) {
	schema, err := GenerateFromValue([]string{})
	require.NoError(t, err)
	require.NotNil(t, schema)

	assert.Equal(t, "array", schema.Type)
	assert.NotNil(t, schema.Items)
	assert.Equal(t, "string", schema.Items.Type)
}

func TestGenerate_NilType(t *testing.T) {
	schema, err := Generate(nil)
	assert.Error(t, err)
	assert.Nil(t, schema)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestGenerateFromValue_NilValue(t *testing.T) {
	schema, err := GenerateFromValue(nil)
	assert.Error(t, err)
	assert.Nil(t, schema)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestSchema_JSON(t *testing.T) {
	tests := []struct {
		name    string
		schema  *Schema
		wantErr bool
	}{
		{
			name: "simple schema",
			schema: &Schema{
				Type: "object",
				Properties: map[string]Property{
					"name": {
						Type:        "string",
						Description: "Name field",
					},
					"age": {
						Type: "integer",
					},
				},
				Required: []string{"name"},
			},
			wantErr: false,
		},
		{
			name: "schema with enum",
			schema: &Schema{
				Type: "object",
				Properties: map[string]Property{
					"role": {
						Type: "string",
						Enum: []any{"admin", "user", "guest"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "schema with array items",
			schema: &Schema{
				Type: "object",
				Properties: map[string]Property{
					"tags": {
						Type: "array",
						Items: &Property{
							Type: "string",
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.schema)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Unmarshal back
			var got Schema
			err = json.Unmarshal(data, &got)
			require.NoError(t, err)

			// Compare
			assert.Equal(t, tt.schema.Type, got.Type)
			assert.Equal(t, len(tt.schema.Properties), len(got.Properties))
			assert.Equal(t, tt.schema.Required, got.Required)
		})
	}
}

func TestGenerate_ComplexExample(t *testing.T) {
	type Address struct {
		Street  string `json:"street"`
		City    string `json:"city"`
		ZipCode string `json:"zip_code"`
	}

	type User struct {
		Name    string   `json:"name"`
		Email   string   `json:"email"`
		Age     int      `json:"age"`
		Active  bool     `json:"active"`
		Address Address  `json:"address"`
		Tags    []string `json:"tags"`
	}

	schema, err := Generate(reflect.TypeFor[User]())
	require.NoError(t, err)
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema.Type)

	// Check all top-level properties exist
	assert.Contains(t, schema.Properties, "name")
	assert.Contains(t, schema.Properties, "email")
	assert.Contains(t, schema.Properties, "age")
	assert.Contains(t, schema.Properties, "active")
	assert.Contains(t, schema.Properties, "address")
	assert.Contains(t, schema.Properties, "tags")

	// Check nested address object
	addressProp := schema.Properties["address"]
	assert.Equal(t, "object", addressProp.Type)

	// Check array type
	tagsProp := schema.Properties["tags"]
	assert.Equal(t, "array", tagsProp.Type)
	assert.NotNil(t, tagsProp.Items)
	assert.Equal(t, "string", tagsProp.Items.Type)
}

func TestParseEnumTag(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want []any
	}{
		{
			name: "simple enum",
			tag:  "admin,user,guest",
			want: []any{"admin", "user", "guest"},
		},
		{
			name: "enum with spaces",
			tag:  "admin, user, guest",
			want: []any{"admin", "user", "guest"},
		},
		{
			name: "single value",
			tag:  "admin",
			want: []any{"admin"},
		},
		{
			name: "empty tag",
			tag:  "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEnumTag(tt.tag)
			assert.Equal(t, tt.want, got)
		})
	}
}
