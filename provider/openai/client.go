// Package openai provides an OpenAI implementation of the provider.Provider
// interface, built on the official SDK and the Responses API.
//
// Requests are stateless: every call sends the full conversation with
// store=false, so nothing is retained on OpenAI's side and Zero Data Retention
// accounts work. For reasoning models, encrypted reasoning items are requested
// and replayed between turns of a run.
//
// Supported models are gpt-4o and newer: the gpt-4o and gpt-4.1 families,
// the o-series, and GPT-5 and later.
package openai

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
)

// DefaultModel is the model used when neither WithModel nor OPENAI_MODEL is set.
const DefaultModel = openai.ChatModelGPT6_1Sol

// Config holds configuration for the OpenAI client.
// Use functional options with New() to configure the client.
type Config struct {
	// Model is the OpenAI model to use (e.g., "gpt-4o", "gpt-4.1", "gpt-6.1-sol").
	// Default: reads from OPENAI_MODEL env var, falls back to DefaultModel
	Model string

	// APIKey is the OpenAI API key for authentication
	// Default: reads from OPENAI_API_KEY env var (required)
	APIKey string

	// BaseURL overrides the API endpoint, e.g. for proxies or compatible servers
	BaseURL string

	// HTTPClient overrides the HTTP client used for requests
	HTTPClient *http.Client

	// ReasoningEffort sets the reasoning effort ("none", "minimal", "low",
	// "medium", "high", "xhigh", "max") for reasoning models. It is ignored
	// for non-reasoning models such as gpt-4o.
	ReasoningEffort string
}

// Option is a functional option for configuring the Client.
type Option func(*Config)

// WithModel sets the OpenAI model to use.
// This overrides the OPENAI_MODEL environment variable.
//
// Example:
//
//	provider, err := openai.New(openai.WithModel("gpt-4o"))
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithAPIKey explicitly sets the API key.
// This overrides the OPENAI_API_KEY environment variable.
//
// Example:
//
//	provider, err := openai.New(openai.WithAPIKey("sk-..."))
func WithAPIKey(apiKey string) Option {
	return func(c *Config) {
		c.APIKey = apiKey
	}
}

// WithBaseURL overrides the API endpoint.
func WithBaseURL(baseURL string) Option {
	return func(c *Config) {
		c.BaseURL = baseURL
	}
}

// WithHTTPClient overrides the HTTP client used for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Config) {
		c.HTTPClient = client
	}
}

// WithReasoningEffort sets the reasoning effort for reasoning models.
// It is ignored for non-reasoning models.
func WithReasoningEffort(effort string) Option {
	return func(c *Config) {
		c.ReasoningEffort = effort
	}
}

// Client implements provider.Provider for OpenAI's Responses API.
type Client struct {
	client          openai.Client
	model           string
	reasoningEffort string
}

// New creates a new OpenAI provider client with functional options.
//
// By default, New reads configuration from environment variables:
//   - OPENAI_API_KEY (required): Your OpenAI API key
//   - OPENAI_MODEL (optional): Model to use, defaults to DefaultModel
//
// Options override the environment.
//
// Examples:
//
//	// Simplest - reads from environment
//	provider, err := openai.New()
//
//	// Use an older, non-reasoning model
//	provider, err := openai.New(openai.WithModel("gpt-4o"))
func New(opts ...Option) (*Client, error) {
	cfg := &Config{
		Model:  os.Getenv("OPENAI_MODEL"),
		APIKey: os.Getenv("OPENAI_API_KEY"),
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf(
			"OpenAI API key required: set OPENAI_API_KEY environment variable " +
				"or use openai.WithAPIKey() option",
		)
	}

	clientOpts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(cfg.BaseURL))
	}
	if cfg.HTTPClient != nil {
		clientOpts = append(clientOpts, option.WithHTTPClient(cfg.HTTPClient))
	}

	return &Client{
		client:          openai.NewClient(clientOpts...),
		model:           cfg.Model,
		reasoningEffort: cfg.ReasoningEffort,
	}, nil
}

// Name returns the model identifier.
func (c *Client) Name() string {
	return c.model
}

// EstimateCost returns the estimated USD cost of usage on this client's model,
// or 0 if the model's price is unknown.
func (c *Client) EstimateCost(usage *message.Usage) float64 {
	return estimateCost(c.model, usage)
}

// Request performs a single Responses API request.
func (c *Client) Request(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	params, err := c.buildParams(req)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Responses.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("responses request failed: %w", err)
	}
	return convertResponse(resp)
}

// Stream performs a streaming Responses API request. It yields text deltas,
// then a final chunk carrying the complete response. Stopping the iteration
// early closes the HTTP stream.
func (c *Client) Stream(ctx context.Context, req *provider.Request) iter.Seq2[*provider.Chunk, error] {
	return func(yield func(*provider.Chunk, error) bool) {
		if req == nil {
			yield(nil, fmt.Errorf("request cannot be nil"))
			return
		}
		params, err := c.buildParams(req)
		if err != nil {
			yield(nil, err)
			return
		}

		stream := c.client.Responses.NewStreaming(ctx, params)
		final, err := forwardEvents(stream, yield)
		if closeErr := stream.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close stream: %w", closeErr)
		}
		switch {
		case errors.Is(err, errStopped):
			return
		case err != nil:
			yield(nil, err)
		default:
			yield(&provider.Chunk{Response: final}, nil)
		}
	}
}

// errStopped reports that the consumer stopped iterating.
var errStopped = errors.New("stream consumer stopped")

// eventStream is the part of the SDK's SSE stream that forwardEvents uses.
type eventStream interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
}

// forwardEvents yields text deltas until a terminal event, which it converts
// to the final response.
func forwardEvents(stream eventStream, yield func(*provider.Chunk, error) bool) (*provider.Response, error) {
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			if !yield(&provider.Chunk{Content: event.Delta}, nil) {
				return nil, errStopped
			}
		case "response.completed", "response.incomplete", "response.failed":
			return convertResponse(&event.Response)
		case "error":
			return nil, fmt.Errorf("stream error %s: %s", event.Code, event.Message)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("stream failed: %w", err)
	}
	return nil, fmt.Errorf("stream ended without a final response")
}
