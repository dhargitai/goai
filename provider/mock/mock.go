// Package mock provides a mock implementation of provider.Provider for testing.
package mock

import (
	"context"
	"fmt"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
)

// Provider is a mock implementation of provider.Provider for testing.
// It allows configuring responses, tool calls, errors, and streaming behavior.
type Provider struct {
	// ModelName is the model identifier to return
	ModelName string

	// Response configures the response for Request() calls
	Response *provider.Response

	// StreamChunks configures the chunks for Stream() calls
	StreamChunks []*provider.Chunk

	// StreamResponse configures the final response for Stream() calls
	StreamResponse *provider.Response

	// Err configures an error to return
	Err error

	// RequestCallback is called when Request() is invoked
	RequestCallback func(ctx context.Context, req *provider.Request)

	// StreamCallback is called when Stream() is invoked
	StreamCallback func(ctx context.Context, req *provider.Request)
}

// New creates a new mock provider with default values.
func New() *Provider {
	return &Provider{
		ModelName: "mock-model",
		Response: &provider.Response{
			Content:      "Mock response",
			FinishReason: "stop",
			Usage: &message.Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		},
	}
}

// Name returns the model identifier.
func (p *Provider) Name() string {
	return p.ModelName
}

// Request performs a mock request.
func (p *Provider) Request(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	if p.RequestCallback != nil {
		p.RequestCallback(ctx, req)
	}

	if p.Err != nil {
		return nil, p.Err
	}

	if p.Response == nil {
		return nil, fmt.Errorf("mock provider: no response configured")
	}

	return p.Response, nil
}

// Stream performs a mock streaming request.
func (p *Provider) Stream(ctx context.Context, req *provider.Request) (*provider.StreamResponse, error) {
	if p.StreamCallback != nil {
		p.StreamCallback(ctx, req)
	}

	if p.Err != nil {
		return nil, p.Err
	}

	// Create cancellable context
	streamCtx, cancel := context.WithCancel(ctx)

	// Create channels
	chunks := make(chan *provider.Chunk, len(p.StreamChunks))
	done := make(chan *provider.Response, 1)
	errCh := make(chan error, 1)

	// Send chunks and response
	go func() {
		defer close(chunks)
		defer close(done)
		defer close(errCh)

		// Send chunks
		for _, chunk := range p.StreamChunks {
			select {
			case <-streamCtx.Done():
				return
			case chunks <- chunk:
			}
		}

		// Send final response
		if p.StreamResponse != nil {
			select {
			case <-streamCtx.Done():
				return
			case done <- p.StreamResponse:
			}
		}
	}()

	resp := &provider.StreamResponse{
		Chunks: chunks,
		Done:   done,
		Err:    errCh,
	}

	// Use reflection to set the cancel function
	// This is a bit of a hack, but necessary since cancel is private
	// In real usage, the StreamResponse would handle this internally
	_ = cancel

	return resp, nil
}

// WithResponse configures the mock response.
func (p *Provider) WithResponse(resp *provider.Response) *Provider {
	p.Response = resp
	return p
}

// WithToolCalls configures tool calls in the response.
func (p *Provider) WithToolCalls(toolCalls []message.ToolCall) *Provider {
	if p.Response == nil {
		p.Response = &provider.Response{}
	}
	p.Response.ToolCalls = toolCalls
	return p
}

// WithError configures an error to return.
func (p *Provider) WithError(err error) *Provider {
	p.Err = err
	return p
}

// WithStreamChunks configures streaming chunks.
func (p *Provider) WithStreamChunks(chunks []*provider.Chunk) *Provider {
	p.StreamChunks = chunks
	return p
}

// WithStreamResponse configures the final streaming response.
func (p *Provider) WithStreamResponse(resp *provider.Response) *Provider {
	p.StreamResponse = resp
	return p
}

// WithRequestCallback sets a callback for Request() calls.
func (p *Provider) WithRequestCallback(fn func(ctx context.Context, req *provider.Request)) *Provider {
	p.RequestCallback = fn
	return p
}

// WithStreamCallback sets a callback for Stream() calls.
func (p *Provider) WithStreamCallback(fn func(ctx context.Context, req *provider.Request)) *Provider {
	p.StreamCallback = fn
	return p
}

// SetStreamResponse is a helper to quickly configure streaming with chunked content.
// It breaks the response into 5-character chunks for testing streaming behavior.
func (p *Provider) SetStreamResponse(content string) {
	// Break content into chunks for realistic streaming
	chunkSize := 5
	var chunks []*provider.Chunk

	for i := 0; i < len(content); i += chunkSize {
		end := min(i+chunkSize, len(content))
		chunks = append(chunks, &provider.Chunk{
			Content: content[i:end],
		})
	}

	// Add finish reason to last chunk
	if len(chunks) > 0 {
		chunks[len(chunks)-1].FinishReason = "stop"
	}

	p.StreamChunks = chunks
	p.StreamResponse = &provider.Response{
		Content:      content,
		FinishReason: "stop",
		Usage: &message.Usage{
			PromptTokens:     10,
			CompletionTokens: len(content) / 4, // Rough token estimate
			TotalTokens:      10 + len(content)/4,
		},
	}
}

// SetStreamToolCall configures a streaming response with a tool call.
// This simulates the model requesting to call a tool.
func (p *Provider) SetStreamToolCall(toolName string, toolArgs string) {
	toolCall := message.ToolCall{
		ID:   "call_123",
		Type: "function",
		Function: message.FunctionCall{
			Name:      toolName,
			Arguments: toolArgs,
		},
	}

	// Send tool call as chunk
	p.StreamChunks = []*provider.Chunk{
		{
			ToolCall: &toolCall,
		},
	}

	p.StreamResponse = &provider.Response{
		ToolCalls:    []message.ToolCall{toolCall},
		FinishReason: "tool_calls",
		Usage: &message.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
}
