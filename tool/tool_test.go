package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test types for tool arguments
type SimpleArgs struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type ComplexArgs struct {
	Query   string   `json:"query"`
	Filters []string `json:"filters"`
	Limit   int      `json:"limit"`
}

// Test handler functions
func simpleHandler(ctx context.Context, args SimpleArgs) (string, error) {
	return "Hello " + args.Name, nil
}

func noArgsHandler(ctx context.Context) (string, error) {
	return "No args", nil
}

func complexHandler(ctx context.Context, args ComplexArgs) (map[string]interface{}, error) {
	return map[string]interface{}{
		"query":   args.Query,
		"count":   len(args.Filters),
		"limited": args.Limit > 0,
	}, nil
}

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	require.NotNil(t, reg)
	assert.Len(t, reg.List(), 0)
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		description string
		handler     any
		wantErr     bool
	}{
		{
			name:        "simple handler",
			toolName:    "greet",
			description: "Greet a person",
			handler:     simpleHandler,
			wantErr:     false,
		},
		{
			name:        "no args handler",
			toolName:    "ping",
			description: "Ping the service",
			handler:     noArgsHandler,
			wantErr:     false,
		},
		{
			name:        "complex handler",
			toolName:    "search",
			description: "Search with filters",
			handler:     complexHandler,
			wantErr:     false,
		},
		{
			name:        "empty name",
			toolName:    "",
			description: "Test",
			handler:     simpleHandler,
			wantErr:     true,
		},
		{
			name:        "not a function",
			toolName:    "bad",
			description: "Test",
			handler:     "not a function",
			wantErr:     true,
		},
		{
			name:        "invalid signature - no context",
			toolName:    "bad",
			description: "Test",
			handler:     func(s string) (string, error) { return s, nil },
			wantErr:     true,
		},
		{
			name:        "invalid signature - wrong return count",
			toolName:    "bad",
			description: "Test",
			handler:     func(ctx context.Context) string { return "test" },
			wantErr:     true,
		},
		{
			name:        "invalid signature - too many params",
			toolName:    "bad",
			description: "Test",
			handler:     func(ctx context.Context, a string, b string) (string, error) { return "", nil },
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry()
			err := reg.Register(tt.toolName, tt.description, tt.handler)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Contains(t, reg.List(), tt.toolName)

			tool := reg.Get(tt.toolName)
			require.NotNil(t, tool)
			assert.Equal(t, tt.toolName, tool.Name)
			assert.Equal(t, tt.description, tool.Description)
			assert.NotNil(t, tool.Schema)
			assert.NotNil(t, tool.Handler)
		})
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("tool1", "First", simpleHandler)
	require.NoError(t, err)

	// Try to register same name again
	err = reg.Register("tool1", "Second", simpleHandler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_GetDefinitions(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("greet", "Greet a person", simpleHandler)
	require.NoError(t, err)

	err = reg.Register("ping", "Ping the service", noArgsHandler)
	require.NoError(t, err)

	defs := reg.GetDefinitions()
	assert.Len(t, defs, 2)

	// Check that definitions have required fields
	for _, def := range defs {
		assert.NotEmpty(t, def.Name)
		assert.NotEmpty(t, def.Description)
		assert.NotNil(t, def.Parameters)
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("greet", "Greet a person", simpleHandler)
	require.NoError(t, err)

	// Test successful execution
	args := SimpleArgs{Name: "Alice", Age: 30}
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	result, err := reg.Execute(context.Background(), "greet", argsJSON)
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice", result)
}

func TestRegistry_Execute_NoArgs(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("ping", "Ping the service", noArgsHandler)
	require.NoError(t, err)

	// Execute with empty args
	result, err := reg.Execute(context.Background(), "ping", nil)
	require.NoError(t, err)
	assert.Equal(t, "No args", result)
}

func TestRegistry_Execute_ComplexArgs(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("search", "Search with filters", complexHandler)
	require.NoError(t, err)

	args := ComplexArgs{
		Query:   "test query",
		Filters: []string{"filter1", "filter2"},
		Limit:   10,
	}
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)

	result, err := reg.Execute(context.Background(), "search", argsJSON)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "test query", resultMap["query"])
	assert.Equal(t, 2, resultMap["count"])
	assert.Equal(t, true, resultMap["limited"])
}

func TestRegistry_Execute_NotFound(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Execute(context.Background(), "nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_Execute_InvalidJSON(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("greet", "Greet a person", simpleHandler)
	require.NoError(t, err)

	// Invalid JSON
	_, err = reg.Execute(context.Background(), "greet", json.RawMessage("invalid json"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register("tool1", "Description", simpleHandler)
	require.NoError(t, err)

	// Existing tool
	tool := reg.Get("tool1")
	require.NotNil(t, tool)
	assert.Equal(t, "tool1", tool.Name)

	// Non-existent tool
	tool = reg.Get("nonexistent")
	assert.Nil(t, tool)
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()

	// Empty registry
	assert.Len(t, reg.List(), 0)

	// Add tools
	err := reg.Register("tool1", "First", simpleHandler)
	require.NoError(t, err)

	err = reg.Register("tool2", "Second", noArgsHandler)
	require.NoError(t, err)

	list := reg.List()
	assert.Len(t, list, 2)
	assert.Contains(t, list, "tool1")
	assert.Contains(t, list, "tool2")
}

func TestCreateHandler_ContextCancellation(t *testing.T) {
	reg := NewRegistry()

	handlerCalled := false
	handler := func(ctx context.Context) (string, error) {
		handlerCalled = true
		// Check if context is properly passed
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			return "success", nil
		}
	}

	err := reg.Register("test", "Test handler", handler)
	require.NoError(t, err)

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = reg.Execute(ctx, "test", nil)
	assert.True(t, handlerCalled)
	// Handler should receive the cancelled context
	assert.Error(t, err)
}

// Benchmark tests
func BenchmarkRegistry_Register(b *testing.B) {
	for i := 0; i < b.N; i++ {
		reg := NewRegistry()
		_ = reg.Register("tool", "desc", simpleHandler)
	}
}

func BenchmarkRegistry_Execute(b *testing.B) {
	reg := NewRegistry()
	_ = reg.Register("greet", "Greet", simpleHandler)

	args := SimpleArgs{Name: "Alice", Age: 30}
	argsJSON, _ := json.Marshal(args)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = reg.Execute(context.Background(), "greet", argsJSON)
	}
}
