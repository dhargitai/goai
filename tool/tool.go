// Package tool provides types and utilities for function calling / tool execution.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dhargitai/goai/schema"
)

// Definition represents a tool/function definition for the LLM.
type Definition struct {
	// Name is the unique identifier for the tool
	Name string `json:"name"`

	// Description explains what the tool does
	Description string `json:"description"`

	// Parameters is the JSON schema for the tool's parameters
	Parameters *schema.Schema `json:"parameters"`
}

// Tool represents a registered tool with its handler.
type Tool struct {
	Name        string
	Description string
	Schema      *schema.Schema
	Handler     Handler
}

// Handler is a generic function signature for tool handlers.
// It receives a context and JSON-encoded arguments, and returns a result and error.
type Handler func(ctx context.Context, args json.RawMessage) (any, error)

// Registry manages a collection of tools and handles execution.
type Registry struct {
	tools map[string]*Tool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*Tool),
	}
}

// Register adds a tool to the registry with automatic schema generation.
// The handler can be any function with the following signature patterns:
//   - func(ctx context.Context, args ArgsStruct) (ResultType, error)
//   - func(ctx context.Context) (ResultType, error)
//
// The function must take context.Context as the first parameter and return
// (result, error) as the last two return values.
func (r *Registry) Register(name, description string, handler any) error {
	// Validate handler is a function
	handlerValue := reflect.ValueOf(handler)
	if handlerValue.Kind() != reflect.Func {
		return fmt.Errorf("handler must be a function, got %s", handlerValue.Kind())
	}
	handlerType := handlerValue.Type()

	// Validate function signature
	if err := validateHandlerSignature(handlerType); err != nil {
		return fmt.Errorf("invalid handler signature: %w", err)
	}

	// Generate schema from parameters (the second parameter after context)
	var argsType reflect.Type
	if handlerType.NumIn() > 1 {
		argsType = handlerType.In(1)
	}
	paramSchema, err := ParamsSchema(argsType)
	if err != nil {
		return err
	}

	return r.RegisterHandler(name, description, paramSchema, createHandler(handlerValue, handlerType))
}

// RegisterHandler adds a tool whose parameter schema is already known.
// It is used by callers that wrap user handlers themselves, such as the agent.
func (r *Registry) RegisterHandler(name, description string, params *schema.Schema, handler Handler) error {
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}
	if r.tools[name] != nil {
		return fmt.Errorf("tool %s already registered", name)
	}
	if params == nil || handler == nil {
		return fmt.Errorf("tool %s needs a parameter schema and a handler", name)
	}

	r.tools[name] = &Tool{
		Name:        name,
		Description: description,
		Schema:      params,
		Handler:     handler,
	}
	return nil
}

// ParamsSchema generates the parameter schema for a tool's argument type.
// A nil argsType means the tool takes no arguments. Tool arguments must be
// a struct, because models always send them as a JSON object.
func ParamsSchema(argsType reflect.Type) (*schema.Schema, error) {
	if argsType == nil {
		return &schema.Schema{Type: "object", Properties: map[string]*schema.Schema{}}, nil
	}
	paramSchema, err := schema.Generate(argsType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}
	if paramSchema.Type != "object" {
		return nil, fmt.Errorf("tool arguments must be a struct, got %s", argsType)
	}
	return paramSchema, nil
}

// GetDefinitions returns tool definitions suitable for passing to the LLM.
func (r *Registry) GetDefinitions() []Definition {
	defs := make([]Definition, 0, len(r.tools))

	for _, tool := range r.tools {
		defs = append(defs, Definition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Schema,
		})
	}

	return defs
}

// Execute runs a tool by name with the given arguments.
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (any, error) {
	tool := r.tools[name]
	if tool == nil {
		return nil, fmt.Errorf("tool %s not found", name)
	}

	return tool.Handler(ctx, args)
}

// Get returns a tool by name, or nil if not found.
func (r *Registry) Get(name string) *Tool {
	return r.tools[name]
}

// List returns all registered tool names.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// validateHandlerSignature validates that the handler function has the correct signature.
func validateHandlerSignature(handlerType reflect.Type) error {
	// Must have at least 1 input (context) and 2 outputs (result, error)
	if handlerType.NumIn() < 1 {
		return fmt.Errorf("handler must have at least 1 parameter (context.Context)")
	}

	if handlerType.NumOut() != 2 {
		return fmt.Errorf("handler must return exactly 2 values (result, error), got %d", handlerType.NumOut())
	}

	// First parameter must be context.Context
	ctxType := reflect.TypeFor[context.Context]()
	if !handlerType.In(0).Implements(ctxType) {
		return fmt.Errorf("first parameter must be context.Context, got %s", handlerType.In(0))
	}

	// Last return value must be error
	errorType := reflect.TypeFor[error]()
	if !handlerType.Out(1).Implements(errorType) {
		return fmt.Errorf("second return value must be error, got %s", handlerType.Out(1))
	}

	// Can have 1 or 2 input parameters (context, or context + args)
	if handlerType.NumIn() > 2 {
		return fmt.Errorf("handler can have at most 2 parameters (context.Context, args), got %d", handlerType.NumIn())
	}

	return nil
}

// createHandler wraps a user-provided handler function into our Handler signature.
func createHandler(handlerValue reflect.Value, handlerType reflect.Type) Handler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		// Prepare input arguments
		inputs := []reflect.Value{reflect.ValueOf(ctx)}

		// If handler expects arguments, unmarshal and add them
		if handlerType.NumIn() > 1 {
			argsType := handlerType.In(1)
			argsPtr := reflect.New(argsType)

			if len(args) > 0 {
				if err := json.Unmarshal(args, argsPtr.Interface()); err != nil {
					return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
				}
			}

			inputs = append(inputs, argsPtr.Elem())
		}

		// Call the handler
		outputs := handlerValue.Call(inputs)

		// Extract result and error
		result := outputs[0].Interface()
		errValue := outputs[1]

		if !errValue.IsNil() {
			return result, errValue.Interface().(error)
		}

		return result, nil
	}
}
