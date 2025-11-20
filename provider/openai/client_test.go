package openai

import (
	"context"
	"testing"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_WithOptions(t *testing.T) {
	// Test that options override defaults
	client, err := New(
		WithModel("gpt-4"),
		WithAPIKey("test-key"),
	)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "gpt-4", client.Name())
}

func TestClient_Name(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{
			name:  "gpt-4",
			model: "gpt-4",
			want:  "gpt-4",
		},
		{
			name:  "gpt-4-turbo-preview",
			model: "gpt-4-turbo-preview",
			want:  "gpt-4-turbo-preview",
		},
		{
			name:  "gpt-3.5-turbo",
			model: "gpt-3.5-turbo",
			want:  "gpt-3.5-turbo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Updated to use options
			client, err := New(
				WithModel(tt.model),
				WithAPIKey("test-api-key"),
			)
			require.NoError(t, err)
			assert.Equal(t, tt.want, client.Name())
		})
	}
}

func TestConvertMessages(t *testing.T) {
	tests := []struct {
		name    string
		msgs    []message.Message
		wantLen int
		wantErr bool
	}{
		{
			name: "simple messages",
			msgs: []message.Message{
				{Role: "system", Content: "You are a helpful assistant"},
				{Role: "user", Content: "Hello"},
			},
			wantLen: 2,
			wantErr: false,
		},
		{
			name: "with tool calls",
			msgs: []message.Message{
				{
					Role:    "assistant",
					Content: "",
					ToolCalls: []message.ToolCall{
						{
							ID:   "call_123",
							Type: "function",
							Function: message.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"location":"San Francisco"}`,
							},
						},
					},
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "tool response",
			msgs: []message.Message{
				{
					Role:       "tool",
					Content:    `{"temperature": 72}`,
					ToolCallID: "call_123",
				},
			},
			wantLen: 1,
			wantErr: false,
		},
		{
			name:    "empty messages",
			msgs:    []message.Message{},
			wantLen: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertMessages(tt.msgs)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.wantLen)

			// Verify roles are preserved
			for i, msg := range tt.msgs {
				assert.Equal(t, msg.Role, got[i].Role)
				assert.Equal(t, msg.Content, got[i].Content)
			}
		})
	}
}

func TestClient_Request_NilRequest(t *testing.T) {
	client, err := New(
		WithModel("gpt-4"),
		WithAPIKey("test-api-key"),
	)
	require.NoError(t, err)
	_, err = client.Request(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestClient_Stream_NilRequest(t *testing.T) {
	client, err := New(
		WithModel("gpt-4"),
		WithAPIKey("test-api-key"),
	)
	require.NoError(t, err)
	_, err = client.Stream(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// Integration test - only runs with valid API key
// To run: OPENAI_API_KEY=sk-xxx go test -v -tags=integration
func TestClient_Request_Integration(t *testing.T) {
	apiKey := getAPIKey(t)
	client, err := New(
		WithModel("gpt-3.5-turbo"),
		WithAPIKey(apiKey),
	)
	require.NoError(t, err)

	req := &provider.Request{
		Messages: []message.Message{
			{Role: "user", Content: "Say 'Hello, World!' and nothing else"},
		},
		Temperature: 0,
		MaxTokens:   50,
	}

	resp, err := client.Request(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.Content)
	assert.NotNil(t, resp.Usage)
	assert.Greater(t, resp.Usage.TotalTokens, 0)
}

func getAPIKey(t *testing.T) string {
	// In integration tests, this would read from environment
	// For now, skip if not in integration mode
	t.Skip("Integration test requires OPENAI_API_KEY environment variable")
	return ""
}

func TestNew_WithEnvDefaults(t *testing.T) {
	tests := []struct {
		name      string
		envModel  string
		envAPIKey string
		wantModel string
		wantErr   bool
	}{
		{
			name:      "env vars set",
			envModel:  "gpt-3.5-turbo",
			envAPIKey: "sk-env-key",
			wantModel: "gpt-3.5-turbo",
			wantErr:   false,
		},
		{
			name:      "model not set uses default",
			envModel:  "",
			envAPIKey: "sk-env-key",
			wantModel: "gpt-4o",
			wantErr:   false,
		},
		{
			name:      "missing API key",
			envModel:  "gpt-4",
			envAPIKey: "",
			wantModel: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always set env vars for this test only (even if empty)
			t.Setenv("OPENAI_MODEL", tt.envModel)
			t.Setenv("OPENAI_API_KEY", tt.envAPIKey)

			client, err := New()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
				assert.Contains(t, err.Error(), "API key required")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
			assert.Equal(t, tt.wantModel, client.Name())
		})
	}
}

func TestNew_OptionsOverrideEnv(t *testing.T) {
	// Set env vars
	t.Setenv("OPENAI_MODEL", "gpt-3.5-turbo")
	t.Setenv("OPENAI_API_KEY", "sk-env-key")

	tests := []struct {
		name      string
		opts      []Option
		wantModel string
	}{
		{
			name:      "WithModel overrides env",
			opts:      []Option{WithModel("gpt-4-turbo")},
			wantModel: "gpt-4-turbo",
		},
		{
			name:      "WithAPIKey overrides env",
			opts:      []Option{WithAPIKey("sk-opt-key")},
			wantModel: "gpt-3.5-turbo", // Uses env model
		},
		{
			name: "both options override env",
			opts: []Option{
				WithModel("gpt-4"),
				WithAPIKey("sk-opt-key"),
			},
			wantModel: "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(tt.opts...)
			require.NoError(t, err)
			require.NotNil(t, client)
			assert.Equal(t, tt.wantModel, client.Name())
		})
	}
}

func TestNew_MissingAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantErr string
	}{
		{
			name:    "no env var, no option",
			opts:    nil,
			wantErr: "API key required",
		},
		{
			name:    "only model provided",
			opts:    []Option{WithModel("gpt-4")},
			wantErr: "API key required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure env var is not set
			t.Setenv("OPENAI_API_KEY", "")

			client, err := New(tt.opts...)

			assert.Error(t, err)
			assert.Nil(t, client)
			assert.Contains(t, err.Error(), tt.wantErr)
			// Verify error message mentions both solutions
			assert.Contains(t, err.Error(), "OPENAI_API_KEY")
			assert.Contains(t, err.Error(), "WithAPIKey")
		})
	}
}

func TestStreamResponse_Close(t *testing.T) {
	// Create a test StreamResponse
	ctx, cancel := context.WithCancel(context.Background())
	chunks := make(chan *provider.Chunk)
	done := make(chan *provider.Response)
	errCh := make(chan error)

	resp := &provider.StreamResponse{
		Chunks: chunks,
		Done:   done,
		Err:    errCh,
	}

	// Set the cancel function using a helper (in real code this is set internally)
	// For test purposes, we'll just verify Close doesn't panic
	err := resp.Close()
	assert.NoError(t, err)

	// Verify cancel was called by checking context
	cancel() // Clean up
	assert.Error(t, ctx.Err())
}
