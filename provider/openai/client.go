// Package openai provides an OpenAI implementation of the provider.Provider interface.
package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/dhargitai/goai/tool"
	openai "github.com/sashabaranov/go-openai"
)

// jsonSchemaWrapper wraps a map to implement json.Marshaler
type jsonSchemaWrapper struct {
	schema map[string]interface{}
}

func (j jsonSchemaWrapper) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.schema)
}

// Config holds configuration for the OpenAI client.
// Use functional options with New() to configure the client.
type Config struct {
	// Model is the OpenAI model to use (e.g., "gpt-4", "gpt-4o", "gpt-3.5-turbo")
	// Default: reads from OPENAI_MODEL env var, falls back to "gpt-4o"
	Model string

	// APIKey is the OpenAI API key for authentication
	// Default: reads from OPENAI_API_KEY env var (required)
	APIKey string
}

// Option is a functional option for configuring the Client.
type Option func(*Config)

// WithModel sets the OpenAI model to use.
// This overrides the OPENAI_MODEL environment variable.
//
// Example:
//
//	provider, err := openai.New(openai.WithModel("gpt-4-turbo"))
func WithModel(model string) Option {
	return func(c *Config) {
		c.Model = model
	}
}

// WithAPIKey explicitly sets the API key.
// This overrides the OPENAI_API_KEY environment variable.
// Useful for testing or when you want to provide the key programmatically.
//
// Example:
//
//	provider, err := openai.New(openai.WithAPIKey("sk-..."))
func WithAPIKey(apiKey string) Option {
	return func(c *Config) {
		c.APIKey = apiKey
	}
}

// Client implements provider.Provider for OpenAI's API.
type Client struct {
	client *openai.Client
	model  string
}

// New creates a new OpenAI provider client with functional options.
//
// By default, New reads configuration from environment variables:
//   - OPENAI_API_KEY (required): Your OpenAI API key
//   - OPENAI_MODEL (optional): Model to use, defaults to "gpt-4o"
//
// You can override these defaults using functional options:
//   - WithModel(model): Override the model
//   - WithAPIKey(key): Override the API key
//
// Examples:
//
//	// Simplest - reads from environment
//	provider, err := openai.New()
//
//	// Override model only
//	provider, err := openai.New(openai.WithModel("gpt-4-turbo"))
//
//	// Override both
//	provider, err := openai.New(
//	    openai.WithModel("gpt-4-turbo"),
//	    openai.WithAPIKey("sk-..."),
//	)
func New(opts ...Option) (*Client, error) {
	// Start with environment variable defaults
	cfg := &Config{
		Model:  os.Getenv("OPENAI_MODEL"),
		APIKey: os.Getenv("OPENAI_API_KEY"),
	}

	// Apply default model if not set
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}

	// Apply all functional options (these override env vars)
	for _, opt := range opts {
		opt(cfg)
	}

	// Validate configuration after all options applied
	if cfg.APIKey == "" {
		return nil, fmt.Errorf(
			"OpenAI API key required: set OPENAI_API_KEY environment variable " +
				"or use openai.WithAPIKey() option",
		)
	}

	// Create and return client (same as before)
	return &Client{
		client: openai.NewClient(cfg.APIKey),
		model:  cfg.Model,
	}, nil
}

// Name returns the model identifier.
func (c *Client) Name() string {
	return c.model
}

// Request performs a single chat completion request.
func (c *Client) Request(ctx context.Context, req *provider.Request) (*provider.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Convert messages
	msgs, err := convertMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: msgs,
	}

	// Set temperature if provided
	if req.Temperature > 0 {
		chatReq.Temperature = float32(req.Temperature)
	}

	// Set max tokens if provided
	if req.MaxTokens > 0 {
		chatReq.MaxTokens = req.MaxTokens
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		chatReq.Tools = convertTools(req.Tools)
	}

	// Add response format if provided
	if req.ResponseFormat != nil {
		respFormat := &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatType(req.ResponseFormat.Type),
		}

		// If using json_schema type, add the schema details
		if req.ResponseFormat.Type == "json_schema" && req.ResponseFormat.JSONSchema != nil {
			// Extract name from the schema if available, otherwise use a default
			name := "response"
			if n, ok := req.ResponseFormat.JSONSchema["name"].(string); ok && n != "" {
				name = n
			}

			respFormat.JSONSchema = &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   name,
				Schema: jsonSchemaWrapper{schema: req.ResponseFormat.JSONSchema},
				Strict: true,
			}
		}

		chatReq.ResponseFormat = respFormat
	}

	// Make request
	resp, err := c.client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	// Convert response
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	result := &provider.Response{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		Usage: &message.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}

	// Convert tool calls if present
	if len(choice.Message.ToolCalls) > 0 {
		result.ToolCalls = make([]message.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			result.ToolCalls[i] = message.ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: message.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return result, nil
}

// Stream performs a streaming chat completion request.
func (c *Client) Stream(ctx context.Context, req *provider.Request) (*provider.StreamResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	// Convert messages
	msgs, err := convertMessages(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:    c.model,
		Messages: msgs,
		Stream:   true,
		// Request usage information in streaming responses
		StreamOptions: &openai.StreamOptions{
			IncludeUsage: true,
		},
	}

	// Set temperature if provided
	if req.Temperature > 0 {
		chatReq.Temperature = float32(req.Temperature)
	}

	// Set max tokens if provided
	if req.MaxTokens > 0 {
		chatReq.MaxTokens = req.MaxTokens
	}

	// Add tools if provided
	if len(req.Tools) > 0 {
		chatReq.Tools = convertTools(req.Tools)
	}

	// Add response format if provided
	if req.ResponseFormat != nil {
		respFormat := &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatType(req.ResponseFormat.Type),
		}

		// If using json_schema type, add the schema details
		if req.ResponseFormat.Type == "json_schema" && req.ResponseFormat.JSONSchema != nil {
			// Extract name from the schema if available, otherwise use a default
			name := "response"
			if n, ok := req.ResponseFormat.JSONSchema["name"].(string); ok && n != "" {
				name = n
			}

			respFormat.JSONSchema = &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   name,
				Schema: jsonSchemaWrapper{schema: req.ResponseFormat.JSONSchema},
				Strict: true,
			}
		}

		chatReq.ResponseFormat = respFormat
	}

	// Create cancellable context for cleanup
	streamCtx, cancel := context.WithCancel(ctx)

	// Create stream
	stream, err := c.client.CreateChatCompletionStream(streamCtx, chatReq)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// Create channels
	chunks := make(chan *provider.Chunk, 10)
	done := make(chan *provider.Response, 1)
	errCh := make(chan error, 1)

	// Start goroutine to process stream
	go func() {
		defer close(chunks)
		defer close(done)
		defer close(errCh)
		defer stream.Close()
		defer cancel()

		var fullContent string
		var toolCalls []message.ToolCall
		var finishReason string
		var usage *message.Usage

		for {
			select {
			case <-streamCtx.Done():
				return
			default:
			}

			chunk, err := stream.Recv()
			if err != nil {
				// EOF means stream is done
				if err.Error() == "EOF" {
					// Send final response
					done <- &provider.Response{
						Content:      fullContent,
						ToolCalls:    toolCalls,
						FinishReason: finishReason,
						Usage:        usage,
					}
					return
				}
				errCh <- fmt.Errorf("stream error: %w", err)
				return
			}

			// Capture usage information if available (may be in chunk without choices)
			if chunk.Usage != nil {
				usage = &message.Usage{
					PromptTokens:     chunk.Usage.PromptTokens,
					CompletionTokens: chunk.Usage.CompletionTokens,
					TotalTokens:      chunk.Usage.TotalTokens,
				}
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			delta := chunk.Choices[0].Delta
			chunkResp := &provider.Chunk{}

			// Accumulate content
			if delta.Content != "" {
				fullContent += delta.Content
				chunkResp.Content = delta.Content
			}

			// Handle tool calls
			if len(delta.ToolCalls) > 0 {
				for _, tc := range delta.ToolCalls {
					// Initialize tool calls array if needed
					if tc.Index != nil && *tc.Index >= len(toolCalls) {
						toolCalls = append(toolCalls, make([]message.ToolCall, *tc.Index-len(toolCalls)+1)...)
					}

					idx := 0
					if tc.Index != nil {
						idx = *tc.Index
					}

					// Update or create tool call
					if tc.ID != "" {
						toolCalls[idx].ID = tc.ID
						toolCalls[idx].Type = string(tc.Type)
					}
					if tc.Function.Name != "" {
						toolCalls[idx].Function.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						toolCalls[idx].Function.Arguments += tc.Function.Arguments
					}

					chunkResp.ToolCall = &toolCalls[idx]
				}
			}

			// Check finish reason
			if chunk.Choices[0].FinishReason != "" {
				finishReason = string(chunk.Choices[0].FinishReason)
				chunkResp.FinishReason = finishReason
			}

			// Send chunk
			select {
			case chunks <- chunkResp:
			case <-streamCtx.Done():
				return
			}
		}
	}()

	return &provider.StreamResponse{
		Chunks: chunks,
		Done:   done,
		Err:    errCh,
	}, nil
}

// convertMessages converts our message format to OpenAI format.
func convertMessages(msgs []message.Message) ([]openai.ChatCompletionMessage, error) {
	result := make([]openai.ChatCompletionMessage, len(msgs))

	for i, msg := range msgs {
		result[i] = openai.ChatCompletionMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		// Handle tool calls
		if len(msg.ToolCalls) > 0 {
			result[i].ToolCalls = make([]openai.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				result[i].ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolType(tc.Type),
					Function: openai.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		// Handle tool call ID for tool responses
		if msg.ToolCallID != "" {
			result[i].ToolCallID = msg.ToolCallID
		}
	}

	return result, nil
}

// convertTools converts our tool definitions to OpenAI format.
func convertTools(tools []tool.Definition) []openai.Tool {
	result := make([]openai.Tool, len(tools))

	for i, t := range tools {
		// Convert parameters to the expected format
		// Ensure properties is always a valid object, not nil
		// Convert to map[string]interface{} to ensure proper JSON serialization
		properties := make(map[string]interface{})
		for k, v := range t.Parameters.Properties {
			properties[k] = v
		}

		// Ensure required is always a valid array, not nil
		required := t.Parameters.Required
		if required == nil {
			required = []string{}
		}

		params := map[string]interface{}{
			"type":       "object",
			"properties": properties,
			"required":   required,
		}

		result[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		}
	}

	return result
}
