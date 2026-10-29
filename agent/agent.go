// Package agent provides the core Agent type and orchestration logic for AI agents.
// It combines providers, tools, and structured outputs into a cohesive interface.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/dhargitai/goai/schema"
	"github.com/dhargitai/goai/tool"
)

// Agent represents an AI agent with dependencies and structured output.
// Deps is the dependency type injected into tool handlers.
// Result is the structured output type expected from the agent.
type Agent[Deps any, Result any] struct {
	// provider is the LLM provider (e.g., OpenAI)
	provider provider.Provider

	// systemPrompt is the instruction set for the agent
	systemPrompt string

	// tools is the registry of available tools
	tools *tool.Registry

	// resultType is the reflect.Type of the Result generic
	resultType reflect.Type

	// resultSchema is the JSON schema for structured output
	resultSchema *schema.Schema

	// deps is the dependency instance injected into tool handlers
	deps Deps
}

// RunContext provides context and dependencies to tool handlers.
// It allows tools to access agent dependencies and usage information.
type RunContext[Deps any] struct {
	// Ctx is the context for the current run
	Ctx context.Context

	// Deps is the dependency instance for this run
	Deps Deps

	// Usage tracks cumulative token usage across tool calls
	Usage *message.Usage
}

// RunResult contains the complete result of an agent run.
type RunResult[Result any] struct {
	// Data is the structured output from the agent
	Data Result

	// Messages is the complete conversation history
	Messages []message.Message

	// Usage contains token usage statistics
	Usage *message.Usage

	// Cost is the estimated cost in USD (if calculable)
	Cost float64
}

// StreamResult represents a streaming agent response.
// It provides channels for receiving incremental chunks and the final result.
// The consumer must call Close() to clean up resources.
type StreamResult[Result any] struct {
	// chunks receives streaming content chunks
	chunks <-chan *StreamChunk

	// done receives the final complete result
	done <-chan *RunResult[Result]

	// err receives any errors during streaming
	err <-chan error

	// cancel is the context cancel function for cleanup
	cancel context.CancelFunc
}

// StreamChunk represents a single streaming chunk from the agent.
type StreamChunk struct {
	// Content is incremental text content
	Content string

	// ToolCall is an incremental tool call (may be partial)
	ToolCall *message.ToolCall

	// IsToolExecution indicates if this chunk is from a tool execution phase
	IsToolExecution bool

	// ToolName is the name of the tool being executed (if IsToolExecution is true)
	ToolName string
}

// Chunks returns the channel for receiving streaming chunks.
func (s *StreamResult[Result]) Chunks() <-chan *StreamChunk {
	return s.chunks
}

// Result blocks until the final result is ready and returns it.
// It returns nil if the stream was closed or an error occurred.
func (s *StreamResult[Result]) Result() (*RunResult[Result], error) {
	select {
	case result := <-s.done:
		return result, nil
	case err := <-s.err:
		return nil, err
	}
}

// Close cancels the streaming request and cleans up resources.
// It should be called when done consuming the stream.
func (s *StreamResult[Result]) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

// NewAgent creates a new agent instance.
// The provider, system prompt, and dependencies must be provided.
// The Result type is inferred from the generic parameter.
func NewAgent[Deps any, Result any](
	prov provider.Provider,
	systemPrompt string,
	deps Deps,
) (*Agent[Deps, Result], error) {
	if prov == nil || reflect.ValueOf(prov).IsNil() {
		return nil, fmt.Errorf("provider cannot be nil")
	}

	if systemPrompt == "" {
		return nil, fmt.Errorf("system prompt cannot be empty")
	}

	// Get the Result type for schema generation
	var resultZero Result
	resultType := reflect.TypeOf(resultZero)

	// Generate JSON schema for Result type
	resultSchema, err := schema.Generate(resultType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate result schema: %w", err)
	}

	return &Agent[Deps, Result]{
		provider:     prov,
		systemPrompt: systemPrompt,
		tools:        tool.NewRegistry(),
		resultType:   resultType,
		resultSchema: resultSchema,
		deps:         deps,
	}, nil
}

// Run executes the agent with the given user prompt.
// It handles message building, provider requests, tool execution loops,
// and structured output parsing.
func (a *Agent[Deps, Result]) Run(ctx context.Context, userPrompt string) (*RunResult[Result], error) {
	if userPrompt == "" {
		return nil, fmt.Errorf("user prompt cannot be empty")
	}

	// Build initial messages
	messages := []message.Message{
		message.NewSystemMessage(a.systemPrompt),
		message.NewUserMessage(userPrompt),
	}

	// Initialize usage tracking
	usage := &message.Usage{}

	// Maximum iterations to prevent infinite loops
	const maxIterations = 10
	iteration := 0

	for iteration < maxIterations {
		iteration++

		// Prepare provider request
		req := &provider.Request{
			Messages: messages,
			Tools:    a.tools.GetDefinitions(),
		}

		// Add structured output format if Result is not string
		if a.resultType.Kind() != reflect.String {
			req.ResponseFormat = &provider.ResponseFormat{
				Type:       "json_schema",
				JSONSchema: convertSchemaToMap(a.resultSchema),
			}
		}

		// Make provider request
		resp, err := a.provider.Request(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("provider request failed (iteration %d): %w", iteration, err)
		}

		// Update usage
		if resp.Usage != nil {
			usage.PromptTokens += resp.Usage.PromptTokens
			usage.CompletionTokens += resp.Usage.CompletionTokens
			usage.TotalTokens += resp.Usage.TotalTokens
		}

		// Check if model wants to call tools
		if len(resp.ToolCalls) > 0 {
			// Add assistant message with tool calls
			messages = append(messages, message.NewAssistantMessageWithToolCalls(resp.ToolCalls))

			// Execute each tool call
			for _, toolCall := range resp.ToolCalls {
				result, err := a.tools.Execute(ctx, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))

				var toolResponse string
				if err != nil {
					toolResponse = fmt.Sprintf("Error: %v", err)
				} else {
					// Convert result to JSON string
					resultJSON, err := json.Marshal(result)
					if err != nil {
						toolResponse = fmt.Sprintf("Error marshaling result: %v", err)
					} else {
						toolResponse = string(resultJSON)
					}
				}

				// Add tool response message
				messages = append(messages, message.NewToolMessage(toolCall.ID, toolResponse))
			}

			// Continue loop to send tool results back to model
			continue
		}

		// No tool calls - this is the final response
		messages = append(messages, message.NewAssistantMessage(resp.Content))

		// Parse structured output from content
		var result Result
		if resp.Content != "" {
			if a.resultType.Kind() == reflect.String {
				// For string results, use content directly
				reflect.ValueOf(&result).Elem().SetString(resp.Content)
			} else {
				// For structured results, parse JSON
				if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
					return nil, fmt.Errorf("failed to parse structured output: %w", err)
				}
			}
		}

		// Calculate cost (simplified, based on OpenAI pricing)
		cost := calculateCost(usage, a.provider.Name())

		return &RunResult[Result]{
			Data:     result,
			Messages: messages,
			Usage:    usage,
			Cost:     cost,
		}, nil
	}

	// Max iterations reached
	return nil, fmt.Errorf("max iterations (%d) reached without completion", maxIterations)
}

// RunStream executes the agent with streaming response.
// It returns a StreamResult that provides channels for incremental updates.
// The caller must call Close() on the StreamResult when done.
func (a *Agent[Deps, Result]) RunStream(ctx context.Context, userPrompt string) (*StreamResult[Result], error) {
	if userPrompt == "" {
		return nil, fmt.Errorf("user prompt cannot be empty")
	}

	// Create channels for streaming
	chunksChan := make(chan *StreamChunk, 10)
	doneChan := make(chan *RunResult[Result], 1)
	errChan := make(chan error, 1)

	// Create cancellable context
	streamCtx, cancel := context.WithCancel(ctx)

	// Start streaming goroutine
	go func() {
		defer close(chunksChan)
		defer close(doneChan)
		defer close(errChan)

		// Build initial messages
		messages := []message.Message{
			message.NewSystemMessage(a.systemPrompt),
			message.NewUserMessage(userPrompt),
		}

		// Initialize usage tracking
		usage := &message.Usage{}

		// Maximum iterations to prevent infinite loops
		const maxIterations = 10
		iteration := 0

		for iteration < maxIterations {
			iteration++

			// Check for cancellation
			select {
			case <-streamCtx.Done():
				errChan <- streamCtx.Err()
				return
			default:
			}

			// Prepare provider request
			req := &provider.Request{
				Messages: messages,
				Tools:    a.tools.GetDefinitions(),
			}

			// Add structured output format if Result is not string
			if a.resultType.Kind() != reflect.String {
				req.ResponseFormat = &provider.ResponseFormat{
					Type:       "json_schema",
					JSONSchema: convertSchemaToMap(a.resultSchema),
				}
			}

			// Make streaming provider request
			streamResp, err := a.provider.Stream(streamCtx, req)
			if err != nil {
				errChan <- fmt.Errorf("provider stream failed (iteration %d): %w", iteration, err)
				return
			}
			defer streamResp.Close()

			// Accumulate response content and tool calls
			var contentBuilder string
			var toolCalls []message.ToolCall

			// Process stream chunks
		streamLoop:
			for {
				select {
				case <-streamCtx.Done():
					errChan <- streamCtx.Err()
					return
				case chunk, ok := <-streamResp.Chunks:
					if !ok {
						break streamLoop
					}

					// Forward content chunks to consumer
					if chunk.Content != "" {
						chunksChan <- &StreamChunk{
							Content: chunk.Content,
						}
						contentBuilder += chunk.Content
					}

					// Accumulate tool calls
					if chunk.ToolCall != nil {
						// Check if this is a new tool call or continuation
						found := false
						for i, tc := range toolCalls {
							if tc.ID == chunk.ToolCall.ID {
								// Append to existing tool call
								toolCalls[i].Function.Arguments += chunk.ToolCall.Function.Arguments
								found = true
								break
							}
						}
						if !found {
							// New tool call
							toolCalls = append(toolCalls, *chunk.ToolCall)
						}
					}

				case resp, ok := <-streamResp.Done:
					if ok && resp != nil {
						// Update usage
						if resp.Usage != nil {
							usage.PromptTokens += resp.Usage.PromptTokens
							usage.CompletionTokens += resp.Usage.CompletionTokens
							usage.TotalTokens += resp.Usage.TotalTokens
						}

						// Use accumulated values if provider doesn't send them separately
						if resp.Content == "" {
							resp.Content = contentBuilder
						}
						if len(resp.ToolCalls) == 0 {
							resp.ToolCalls = toolCalls
						}
						contentBuilder = resp.Content
						toolCalls = resp.ToolCalls
					}
					break streamLoop

				case err, ok := <-streamResp.Err:
					if ok && err != nil {
						errChan <- fmt.Errorf("streaming error: %w", err)
						return
					}
					break streamLoop
				}
			}

			// Check if model wants to call tools
			if len(toolCalls) > 0 {
				// Add assistant message with tool calls
				messages = append(messages, message.NewAssistantMessageWithToolCalls(toolCalls))

				// Execute each tool call
				for _, toolCall := range toolCalls {
					// Notify consumer about tool execution
					chunksChan <- &StreamChunk{
						IsToolExecution: true,
						ToolName:        toolCall.Function.Name,
					}

					result, err := a.tools.Execute(streamCtx, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))

					var toolResponse string
					if err != nil {
						toolResponse = fmt.Sprintf("Error: %v", err)
					} else {
						// Convert result to JSON string
						resultJSON, err := json.Marshal(result)
						if err != nil {
							toolResponse = fmt.Sprintf("Error marshaling result: %v", err)
						} else {
							toolResponse = string(resultJSON)
						}
					}

					// Add tool response message
					messages = append(messages, message.NewToolMessage(toolCall.ID, toolResponse))
				}

				// Continue loop to send tool results back to model
				continue
			}

			// No tool calls - this is the final response
			messages = append(messages, message.NewAssistantMessage(contentBuilder))

			// Parse structured output from content
			var result Result
			if contentBuilder != "" {
				if a.resultType.Kind() == reflect.String {
					// For string results, use content directly
					reflect.ValueOf(&result).Elem().SetString(contentBuilder)
				} else {
					// For structured results, parse JSON
					if err := json.Unmarshal([]byte(contentBuilder), &result); err != nil {
						errChan <- fmt.Errorf("failed to parse structured output: %w", err)
						return
					}
				}
			}

			// Calculate cost
			cost := calculateCost(usage, a.provider.Name())

			// Send final result
			doneChan <- &RunResult[Result]{
				Data:     result,
				Messages: messages,
				Usage:    usage,
				Cost:     cost,
			}
			return
		}

		// Max iterations reached
		errChan <- fmt.Errorf("max iterations (%d) reached without completion", maxIterations)
	}()

	return &StreamResult[Result]{
		chunks: chunksChan,
		done:   doneChan,
		err:    errChan,
		cancel: cancel,
	}, nil
}

// Tool registers a tool with the agent.
// The handler function should have the signature:
//
//	func(ctx RunContext[Deps], args ArgsType) (ResultType, error)
//
// or:
//
//	func(ctx RunContext[Deps]) (ResultType, error)
//
// The function will be registered with automatic schema generation.
func (a *Agent[Deps, Result]) Tool(name, description string, handler any) error {
	// Validate handler is a function
	handlerValue := reflect.ValueOf(handler)
	handlerType := handlerValue.Type()

	if handlerType.Kind() != reflect.Func {
		return fmt.Errorf("handler must be a function, got %s", handlerType.Kind())
	}

	// Create wrapper that provides RunContext instead of just context.Context
	wrappedHandler := a.createToolHandler(handlerValue, handlerType)

	// Register with tool registry
	return a.tools.Register(name, description, wrappedHandler)
}

// createToolHandler wraps a user handler to inject RunContext.
func (a *Agent[Deps, Result]) createToolHandler(handlerValue reflect.Value, handlerType reflect.Type) any {
	// Determine the argument type (if any)
	var argsType reflect.Type
	if handlerType.NumIn() > 1 {
		argsType = handlerType.In(1)
	}

	// Create wrapper function with correct signature
	wrapper := func(ctx context.Context, args json.RawMessage) (any, error) {
		// Create RunContext
		runCtx := RunContext[Deps]{
			Ctx:   ctx,
			Deps:  a.deps,
			Usage: &message.Usage{},
		}

		// Prepare inputs
		inputs := []reflect.Value{reflect.ValueOf(runCtx)}

		// Add args if handler expects them
		if argsType != nil {
			argsPtr := reflect.New(argsType)
			if len(args) > 0 {
				if err := json.Unmarshal(args, argsPtr.Interface()); err != nil {
					return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
				}
			}
			inputs = append(inputs, argsPtr.Elem())
		}

		// Call handler
		outputs := handlerValue.Call(inputs)

		// Extract result and error
		result := outputs[0].Interface()
		errValue := outputs[1]

		if !errValue.IsNil() {
			return result, errValue.Interface().(error)
		}

		return result, nil
	}

	return wrapper
}

// convertSchemaToMap converts a Schema to a map for provider requests.
func convertSchemaToMap(s *schema.Schema) map[string]any {
	result := make(map[string]any)

	if s.Type != "" {
		result["type"] = s.Type
		// OpenAI strict mode requires additionalProperties: false for all objects
		if s.Type == "object" {
			result["additionalProperties"] = false
		}
	}

	if s.Description != "" {
		result["description"] = s.Description
	}

	if len(s.Properties) > 0 {
		props := make(map[string]any)
		// OpenAI strict mode requires all properties to be in required array
		allRequired := make([]string, 0, len(s.Properties))

		for key, prop := range s.Properties {
			propMap := make(map[string]any)
			if prop.Type != "" {
				propMap["type"] = prop.Type
				// OpenAI strict mode requires additionalProperties: false for all objects
				if prop.Type == "object" {
					propMap["additionalProperties"] = false
				}
			}
			if prop.Description != "" {
				propMap["description"] = prop.Description
			}
			if len(prop.Enum) > 0 {
				propMap["enum"] = prop.Enum
			}
			if prop.Items != nil {
				itemMap := make(map[string]any)
				if prop.Items.Type != "" {
					itemMap["type"] = prop.Items.Type
					// OpenAI strict mode requires additionalProperties: false for all objects
					if prop.Items.Type == "object" {
						itemMap["additionalProperties"] = false
					}
				}
				propMap["items"] = itemMap
			}
			props[key] = propMap
			allRequired = append(allRequired, key)
		}
		result["properties"] = props
		// For OpenAI structured outputs, all properties must be required
		result["required"] = allRequired
	}

	if s.Items != nil {
		result["items"] = convertSchemaToMap(s.Items)
	}

	if len(s.Enum) > 0 {
		result["enum"] = s.Enum
	}

	return result
}

// calculateCost estimates the cost based on token usage and model name.
// Pricing is simplified and based on OpenAI's pricing as of 2025.
func calculateCost(usage *message.Usage, modelName string) float64 {
	if usage == nil {
		return 0.0
	}

	// Simplified pricing (per 1M tokens)
	var inputCost, outputCost float64

	// Default to GPT-4o pricing
	inputCost = 2.50  // $2.50 per 1M input tokens
	outputCost = 10.0 // $10.00 per 1M output tokens

	// Calculate cost in USD
	input := float64(usage.PromptTokens) / 1_000_000.0 * inputCost
	output := float64(usage.CompletionTokens) / 1_000_000.0 * outputCost

	return input + output
}
