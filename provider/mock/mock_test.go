package mock

import (
	"context"
	"testing"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	p := New()
	require.NotNil(t, p)
	assert.Equal(t, "mock-model", p.Name())
	assert.NotNil(t, p.Response)
}

func TestProvider_Name(t *testing.T) {
	p := &Provider{ModelName: "test-model"}
	assert.Equal(t, "test-model", p.Name())
}

func TestProvider_Request(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Provider)
		wantErr bool
	}{
		{
			name: "successful request",
			setup: func(p *Provider) {
				p.Response = &provider.Response{
					Content:      "Test response",
					FinishReason: "stop",
				}
			},
			wantErr: false,
		},
		{
			name: "error response",
			setup: func(p *Provider) {
				p.Err = assert.AnError
			},
			wantErr: true,
		},
		{
			name: "no response configured",
			setup: func(p *Provider) {
				p.Response = nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			tt.setup(p)

			req := &provider.Request{
				Messages: []message.Message{
					{Role: "user", Content: "test"},
				},
			}

			resp, err := p.Request(context.Background(), req)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.NotEmpty(t, resp.Content)
		})
	}
}

func TestProvider_Request_WithCallback(t *testing.T) {
	var capturedReq *provider.Request

	p := New()
	p.RequestCallback = func(ctx context.Context, req *provider.Request) {
		capturedReq = req
	}

	req := &provider.Request{
		Messages: []message.Message{
			{Role: "user", Content: "test"},
		},
	}

	_, err := p.Request(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, req, capturedReq)
}

func TestProvider_Stream(t *testing.T) {
	p := New()
	p.StreamChunks = []*provider.Chunk{
		{Content: "Hello"},
		{Content: " "},
		{Content: "World"},
	}
	p.StreamResponse = &provider.Response{
		Content:      "Hello World",
		FinishReason: "stop",
	}

	req := &provider.Request{
		Messages: []message.Message{
			{Role: "user", Content: "test"},
		},
	}

	resp, err := p.Stream(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Collect chunks
	var chunks []*provider.Chunk
	for chunk := range resp.Chunks {
		chunks = append(chunks, chunk)
	}

	assert.Len(t, chunks, 3)
	assert.Equal(t, "Hello", chunks[0].Content)
	assert.Equal(t, " ", chunks[1].Content)
	assert.Equal(t, "World", chunks[2].Content)

	// Get final response
	finalResp := <-resp.Done
	require.NotNil(t, finalResp)
	assert.Equal(t, "Hello World", finalResp.Content)
}

func TestProvider_Stream_WithError(t *testing.T) {
	p := New()
	p.Err = assert.AnError

	req := &provider.Request{
		Messages: []message.Message{
			{Role: "user", Content: "test"},
		},
	}

	_, err := p.Stream(context.Background(), req)
	assert.Error(t, err)
}

func TestProvider_Stream_WithCallback(t *testing.T) {
	var capturedReq *provider.Request

	p := New()
	p.StreamCallback = func(ctx context.Context, req *provider.Request) {
		capturedReq = req
	}
	p.StreamChunks = []*provider.Chunk{{Content: "test"}}
	p.StreamResponse = &provider.Response{Content: "test"}

	req := &provider.Request{
		Messages: []message.Message{
			{Role: "user", Content: "test"},
		},
	}

	resp, err := p.Stream(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Drain channels
	for range resp.Chunks {
	}
	<-resp.Done

	assert.Equal(t, req, capturedReq)
}

func TestProvider_WithResponse(t *testing.T) {
	p := New()
	customResp := &provider.Response{
		Content: "Custom response",
	}

	p.WithResponse(customResp)
	assert.Equal(t, customResp, p.Response)
}

func TestProvider_WithToolCalls(t *testing.T) {
	p := New()
	toolCalls := []message.ToolCall{
		{
			ID:   "call_123",
			Type: "function",
			Function: message.FunctionCall{
				Name:      "test_tool",
				Arguments: `{"arg":"value"}`,
			},
		},
	}

	p.WithToolCalls(toolCalls)
	assert.Equal(t, toolCalls, p.Response.ToolCalls)
}

func TestProvider_WithError(t *testing.T) {
	p := New()
	testErr := assert.AnError

	p.WithError(testErr)
	assert.Equal(t, testErr, p.Err)
}

func TestProvider_WithStreamChunks(t *testing.T) {
	p := New()
	chunks := []*provider.Chunk{
		{Content: "chunk1"},
		{Content: "chunk2"},
	}

	p.WithStreamChunks(chunks)
	assert.Equal(t, chunks, p.StreamChunks)
}

func TestProvider_WithStreamResponse(t *testing.T) {
	p := New()
	resp := &provider.Response{
		Content: "Final response",
	}

	p.WithStreamResponse(resp)
	assert.Equal(t, resp, p.StreamResponse)
}

func TestProvider_ChainedConfiguration(t *testing.T) {
	// Test that fluent API works correctly
	p := New().
		WithResponse(&provider.Response{Content: "test"}).
		WithError(assert.AnError).
		WithStreamChunks([]*provider.Chunk{{Content: "chunk"}})

	assert.NotNil(t, p.Response)
	assert.NotNil(t, p.Err)
	assert.Len(t, p.StreamChunks, 1)
}
