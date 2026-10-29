package tool

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhargitai/goai/schema"
)

func echoHandler(ctx context.Context, args json.RawMessage) (any, error) {
	return string(args), nil
}

func TestRegistry_RegisterHandler(t *testing.T) {
	params := &schema.Schema{Type: "object"}
	tests := []struct {
		name    string
		tool    string
		params  *schema.Schema
		handler Handler
		wantErr string
	}{
		{name: "valid", tool: "echo", params: params, handler: echoHandler},
		{name: "empty name", tool: "", params: params, handler: echoHandler, wantErr: "name cannot be empty"},
		{name: "nil schema", tool: "echo", params: nil, handler: echoHandler, wantErr: "needs a parameter schema"},
		{name: "nil handler", tool: "echo", params: params, handler: nil, wantErr: "needs a parameter schema and a handler"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			err := r.RegisterHandler(tt.tool, "Echo", tt.params, tt.handler)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Same(t, params, r.Get(tt.tool).Schema)

			err = r.RegisterHandler(tt.tool, "Echo", tt.params, tt.handler)
			assert.ErrorContains(t, err, "already registered")
		})
	}
}

func TestParamsSchema(t *testing.T) {
	noArgs, err := ParamsSchema(nil)
	require.NoError(t, err)
	assert.Equal(t, "object", noArgs.Type)
	assert.Empty(t, noArgs.Properties)

	structArgs, err := ParamsSchema(reflect.TypeFor[SimpleArgs]())
	require.NoError(t, err)
	assert.Contains(t, structArgs.Properties, "name")

	_, err = ParamsSchema(reflect.TypeFor[string]())
	assert.ErrorContains(t, err, "must be a struct")

	_, err = ParamsSchema(reflect.TypeFor[chan int]())
	assert.ErrorContains(t, err, "failed to generate schema")
}
