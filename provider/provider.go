// Package provider defines the interface for LLM providers and related types.
// It provides abstractions for model requests, responses and streaming.
package provider

import (
	"context"
	"encoding/json"
	"iter"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/schema"
	"github.com/dhargitai/goai/tool"
)

// Provider defines the interface that all LLM providers must implement.
// It supports both single-shot requests and streaming responses.
type Provider interface {
	// Request performs a single model request.
	// It returns the complete response or an error.
	Request(ctx context.Context, req *Request) (*Response, error)

	// Stream performs a streaming model request. It yields text chunks as they
	// arrive; the last chunk carries the complete Response. Stopping the
	// iteration early cancels the underlying request.
	Stream(ctx context.Context, req *Request) iter.Seq2[*Chunk, error]

	// Name returns the model identifier for this provider.
	Name() string
}

// CostEstimator is optionally implemented by providers that know their
// model's pricing.
type CostEstimator interface {
	// EstimateCost returns the estimated cost of usage in USD, or 0 if unknown.
	EstimateCost(usage *message.Usage) float64
}

// Request represents a request to an LLM provider.
type Request struct {
	// Messages is the conversation history
	Messages []message.Message

	// Tools is the list of available functions/tools
	Tools []tool.Definition

	// Temperature controls randomness (0.0-2.0, higher = more random).
	// Zero leaves the model default. Providers may ignore it for models
	// that do not support it.
	Temperature float64

	// MaxTokens is the maximum number of tokens to generate. Zero means no limit.
	MaxTokens int

	// ResponseFormat optionally requests structured output matching a schema
	ResponseFormat *ResponseFormat
}

// Response represents a completed model response.
type Response struct {
	// Content is the text response from the model
	Content string

	// ToolCalls contains any function/tool calls requested by the model
	ToolCalls []message.ToolCall

	// ProviderItems holds opaque provider-specific items (such as encrypted
	// reasoning) that must be sent back with the next request of the same run
	ProviderItems []json.RawMessage

	// Usage contains token usage statistics
	Usage *message.Usage
}

// Chunk is a single streaming update. Content carries incremental text;
// Response is set only on the final chunk.
type Chunk struct {
	// Content is the incremental text content
	Content string

	// Response is the complete response, set on the final chunk only
	Response *Response
}

// ResponseFormat requests structured JSON output matching a schema.
type ResponseFormat struct {
	// Name identifies the output schema to the model
	Name string

	// Schema is the JSON schema the output must follow
	Schema *schema.Schema
}
