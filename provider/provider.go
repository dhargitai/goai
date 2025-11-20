// Package provider defines the interface for LLM providers and related types.
// It provides abstractions for chat completion requests and responses.
package provider

import (
	"context"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/tool"
)

// Provider defines the interface that all LLM providers must implement.
// It supports both single-shot requests and streaming responses.
type Provider interface {
	// Request performs a single chat completion request.
	// It returns the complete response or an error.
	Request(ctx context.Context, req *Request) (*Response, error)

	// Stream performs a streaming chat completion request.
	// It returns a StreamResponse with channels for chunks, final result, and errors.
	Stream(ctx context.Context, req *Request) (*StreamResponse, error)

	// Name returns the model identifier for this provider.
	Name() string
}

// Request represents a chat completion request to an LLM provider.
type Request struct {
	// Messages is the conversation history
	Messages []message.Message

	// Tools is the list of available functions/tools
	Tools []tool.Definition

	// Temperature controls randomness (0.0-2.0, higher = more random)
	Temperature float64

	// MaxTokens is the maximum number of tokens to generate
	MaxTokens int

	// ResponseFormat optionally specifies JSON schema for structured output
	ResponseFormat *ResponseFormat
}

// Response represents a completed chat completion response.
type Response struct {
	// Content is the text response from the model
	Content string

	// ToolCalls contains any function/tool calls requested by the model
	ToolCalls []message.ToolCall

	// Usage contains token usage statistics
	Usage *message.Usage

	// FinishReason indicates why the model stopped generating
	// (e.g., "stop", "length", "tool_calls", "content_filter")
	FinishReason string
}

// StreamResponse represents a streaming chat completion response.
// It provides channels for receiving chunks, the final response, and errors.
// The consumer must call Close() to clean up resources.
type StreamResponse struct {
	// Chunks channel receives streaming content chunks
	Chunks <-chan *Chunk

	// Done channel receives the final complete response
	Done <-chan *Response

	// Err channel receives any errors during streaming
	Err <-chan error

	// cancel is the context cancel function for cleanup
	cancel context.CancelFunc
}

// Close cancels the streaming request and cleans up resources.
// It should be called when done consuming the stream.
func (s *StreamResponse) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// Chunk represents a single streaming response chunk.
type Chunk struct {
	// Content is the incremental text content
	Content string

	// ToolCall is an incremental tool call (may be partial)
	ToolCall *message.ToolCall

	// FinishReason is set on the final chunk
	FinishReason string
}

// ResponseFormat specifies the format for structured output.
type ResponseFormat struct {
	// Type is typically "json_object" or "json_schema"
	Type string

	// JSONSchema is the schema for structured output (optional)
	JSONSchema map[string]interface{}
}
