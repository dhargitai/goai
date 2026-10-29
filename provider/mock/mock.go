// Package mock provides a mock implementation of provider.Provider for testing.
package mock

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
)

// defaultChunkSize is how many bytes of content each streamed chunk carries.
const defaultChunkSize = 5

// Provider is a mock implementation of provider.Provider for testing.
// Request and Stream return the queued Responses in order; once the queue is
// empty they keep returning Response.
type Provider struct {
	// ModelName is the model identifier to return
	ModelName string

	// Response is returned when the Responses queue is empty
	Response *provider.Response

	// Responses is a queue of responses returned by successive calls
	Responses []*provider.Response

	// Err configures an error to return
	Err error

	// ChunkSize is how many bytes of content each streamed chunk carries
	ChunkSize int

	// RequestCallback is called on every Request() and Stream() call
	RequestCallback func(ctx context.Context, req *provider.Request)

	// Requests records every request received, in order
	Requests []*provider.Request

	mu sync.Mutex
}

// New creates a new mock provider with default values.
func New() *Provider {
	return &Provider{
		ModelName: "mock-model",
		ChunkSize: defaultChunkSize,
		Response: &provider.Response{
			Content: "Mock response",
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
	return p.next(ctx, req)
}

// Stream performs a mock streaming request. It yields the response content in
// ChunkSize pieces, then a final chunk carrying the whole response.
func (p *Provider) Stream(ctx context.Context, req *provider.Request) iter.Seq2[*provider.Chunk, error] {
	return func(yield func(*provider.Chunk, error) bool) {
		resp, err := p.next(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, piece := range splitContent(resp.Content, p.ChunkSize) {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			if !yield(&provider.Chunk{Content: piece}, nil) {
				return
			}
		}
		yield(&provider.Chunk{Response: resp}, nil)
	}
}

func (p *Provider) next(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	if p.RequestCallback != nil {
		p.RequestCallback(ctx, req)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.Requests = append(p.Requests, req)

	if p.Err != nil {
		return nil, p.Err
	}
	if len(p.Responses) > 0 {
		resp := p.Responses[0]
		p.Responses = p.Responses[1:]
		return resp, nil
	}
	if p.Response == nil {
		return nil, fmt.Errorf("mock provider: no response configured")
	}
	return p.Response, nil
}

func splitContent(content string, size int) []string {
	if size <= 0 {
		size = defaultChunkSize
	}
	var pieces []string
	for i := 0; i < len(content); i += size {
		pieces = append(pieces, content[i:min(i+size, len(content))])
	}
	return pieces
}

// WithResponse configures the response returned once the queue is empty.
func (p *Provider) WithResponse(resp *provider.Response) *Provider {
	p.Response = resp
	return p
}

// WithResponses queues responses returned by successive calls.
func (p *Provider) WithResponses(resps ...*provider.Response) *Provider {
	p.Responses = append(p.Responses, resps...)
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

// WithRequestCallback sets a callback for Request() and Stream() calls.
func (p *Provider) WithRequestCallback(fn func(ctx context.Context, req *provider.Request)) *Provider {
	p.RequestCallback = fn
	return p
}

// SetStreamResponse configures a text response with rough usage numbers.
func (p *Provider) SetStreamResponse(content string) {
	p.Response = &provider.Response{
		Content: content,
		Usage: &message.Usage{
			PromptTokens:     10,
			CompletionTokens: len(content) / 4, // Rough token estimate
			TotalTokens:      10 + len(content)/4,
		},
	}
}

// SetStreamToolCall configures a response in which the model calls a tool.
func (p *Provider) SetStreamToolCall(toolName string, toolArgs string) {
	p.Response = &provider.Response{
		ToolCalls: []message.ToolCall{{
			ID:   "call_123",
			Type: "function",
			Function: message.FunctionCall{
				Name:      toolName,
				Arguments: toolArgs,
			},
		}},
		Usage: &message.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
}
