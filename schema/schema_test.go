package schema

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

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

type StructWithEnum struct {
	Role     string `json:"role" enum:"admin,user,guest"`
	Priority int    `json:"priority" enum:"1, 2, 3"`
}

type Location struct {
	City    string `json:"city" description:"City name"`
	Country string `json:"country"`
}

type Weather struct {
	Location Location   `json:"location"`
	History  []Location `json:"history"`
	Temp     float64    `json:"temp"`
}

type StructWithOptional struct {
	Name     string    `json:"name"`
	Nickname *string   `json:"nickname"`
	Note     string    `json:"note,omitempty"`
	Level    *string   `json:"level" enum:"low,high"`
	At       time.Time `json:"at"`
	Ignored  string    `json:"-"`
	private  string
}

type Base struct {
	ID string `json:"id"`
}

type StructWithEmbedded struct {
	Base
	Name string `json:"name"`
}

type Recursive struct {
	Children []Recursive `json:"children"`
}

func TestGenerate_SimpleStruct(t *testing.T) {
	s, err := Generate(reflect.TypeFor[SimpleStruct]())
	require.NoError(t, err)

	assert.Equal(t, "object", s.Type)
	assert.Equal(t, "string", s.Properties["name"].Type)
	assert.Equal(t, "integer", s.Properties["age"].Type)
	assert.Equal(t, []string{"name", "age"}, s.Required)
}

func TestGenerate_DescriptionTag(t *testing.T) {
	s, err := Generate(reflect.TypeFor[StructWithDescription]())
	require.NoError(t, err)

	assert.Equal(t, "Person's full name", s.Properties["name"].Description)
	assert.Equal(t, "Email address", s.Properties["email"].Description)
}

func TestGenerate_EnumTag(t *testing.T) {
	s, err := Generate(reflect.TypeFor[StructWithEnum]())
	require.NoError(t, err)

	assert.Equal(t, []any{"admin", "user", "guest"}, s.Properties["role"].Enum)
	assert.Equal(t, []any{int64(1), int64(2), int64(3)}, s.Properties["priority"].Enum)
}

func TestGenerate_InvalidEnumValue(t *testing.T) {
	type bad struct {
		N int `json:"n" enum:"one"`
	}
	_, err := Generate(reflect.TypeFor[bad]())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid enum value")
}

// Regression: nested struct fields used to be collapsed into an empty object,
// so strict structured output could not return them.
func TestGenerate_NestedStructKeepsFields(t *testing.T) {
	s, err := Generate(reflect.TypeFor[Weather]())
	require.NoError(t, err)

	tests := []struct {
		name string
		got  *Schema
	}{
		{name: "nested struct", got: s.Properties["location"]},
		{name: "slice of structs", got: s.Properties["history"].Items},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.got)
			assert.Equal(t, "object", tt.got.Type)
			require.Contains(t, tt.got.Properties, "city")
			assert.Equal(t, "string", tt.got.Properties["city"].Type)
			assert.Equal(t, "City name", tt.got.Properties["city"].Description)
			assert.Equal(t, []string{"city", "country"}, tt.got.Required)
		})
	}
}

func TestGenerate_FieldHandling(t *testing.T) {
	s, err := GenerateFromValue(StructWithOptional{Name: "n", private: "hidden"})
	require.NoError(t, err)

	assert.Equal(t, []string{"name", "at"}, s.Required)
	assert.NotContains(t, s.Properties, "Ignored")
	assert.NotContains(t, s.Properties, "private")
	assert.Equal(t, "string", s.Properties["nickname"].Type)
	assert.Equal(t, "date-time", s.Properties["at"].Format)
}

func TestGenerate_EmbeddedStructIsFlattened(t *testing.T) {
	s, err := Generate(reflect.TypeFor[StructWithEmbedded]())
	require.NoError(t, err)

	assert.Contains(t, s.Properties, "id")
	assert.Contains(t, s.Properties, "name")
	assert.NotContains(t, s.Properties, "Base")
}

func TestGenerate_RecursiveTypeErrors(t *testing.T) {
	_, err := Generate(reflect.TypeFor[Recursive]())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recursive type")
}

func TestGenerate_NonStructTypes(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		wantType  string
		wantItems string
	}{
		{name: "string", value: "", wantType: "string"},
		{name: "int", value: 0, wantType: "integer"},
		{name: "float64", value: 0.0, wantType: "number"},
		{name: "bool", value: false, wantType: "boolean"},
		{name: "string slice", value: []string{}, wantType: "array", wantItems: "string"},
		{name: "byte slice", value: []byte{}, wantType: "string"},
		{name: "map", value: map[string]int{}, wantType: "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := GenerateFromValue(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, s.Type)
			if tt.wantItems != "" {
				require.NotNil(t, s.Items)
				assert.Equal(t, tt.wantItems, s.Items.Type)
			}
		})
	}
}

func TestGenerate_NilInputs(t *testing.T) {
	_, err := Generate(nil)
	assert.ErrorContains(t, err, "cannot be nil")

	_, err = GenerateFromValue(nil)
	assert.ErrorContains(t, err, "cannot be nil")
}

func TestGenerate_UnsupportedType(t *testing.T) {
	_, err := GenerateFromValue(make(chan int))
	assert.ErrorContains(t, err, "unsupported type")
}

func TestSchema_Strict(t *testing.T) {
	s, err := Generate(reflect.TypeFor[StructWithOptional]())
	require.NoError(t, err)

	strict := s.Strict()

	require.NotNil(t, strict.AdditionalProperties)
	assert.False(t, *strict.AdditionalProperties)
	assert.ElementsMatch(t, []string{"name", "nickname", "note", "level", "at"}, strict.Required)
	assert.False(t, strict.Properties["name"].Nullable)
	assert.True(t, strict.Properties["nickname"].Nullable)
	assert.True(t, strict.Properties["note"].Nullable)
	assert.Equal(t, []any{"low", "high", nil}, strict.Properties["level"].Enum)

	// The original schema is not modified.
	assert.Equal(t, []string{"name", "at"}, s.Required)
	assert.False(t, s.Properties["nickname"].Nullable)
	assert.Nil(t, s.AdditionalProperties)
}

func TestSchema_StrictNested(t *testing.T) {
	s, err := Generate(reflect.TypeFor[Weather]())
	require.NoError(t, err)

	strict := s.Strict()
	for _, nested := range []*Schema{strict.Properties["location"], strict.Properties["history"].Items} {
		require.NotNil(t, nested.AdditionalProperties)
		assert.False(t, *nested.AdditionalProperties)
		assert.ElementsMatch(t, []string{"city", "country"}, nested.Required)
	}
}

func TestSchema_MarshalJSON(t *testing.T) {
	closed := false
	tests := []struct {
		name   string
		schema *Schema
		want   string
	}{
		{
			name:   "nullable string",
			schema: &Schema{Type: "string", Nullable: true},
			want:   `{"type":["string","null"]}`,
		},
		{
			name:   "empty object keeps properties and required",
			schema: &Schema{Type: "object", AdditionalProperties: &closed},
			want:   `{"type":"object","properties":{},"required":[],"additionalProperties":false}`,
		},
		{
			name:   "untyped",
			schema: &Schema{},
			want:   `{}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.schema)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestSchema_JSONRoundTrip(t *testing.T) {
	s, err := Generate(reflect.TypeFor[StructWithOptional]())
	require.NoError(t, err)
	strict := s.Strict()

	data, err := json.Marshal(strict)
	require.NoError(t, err)

	var got Schema
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, strict, &got)
}

func TestSchema_UnmarshalInvalidType(t *testing.T) {
	var s Schema
	err := json.Unmarshal([]byte(`{"type":42}`), &s)
	assert.ErrorContains(t, err, "invalid schema type")
}
