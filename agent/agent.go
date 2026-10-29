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

// maxIterations caps model round-trips per run to prevent infinite tool loops.
const maxIterations = 10

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

	// resultIsString reports whether Result is a string kind, which is
	// returned as plain text instead of structured JSON
	resultIsString bool

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

	// Usage is the token usage of the current run so far. Treat it as read-only.
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

	// Cost is the estimated cost in USD, or 0 if the provider does not
	// implement provider.CostEstimator or does not know the model's price
	Cost float64
}

// NewAgent creates a new agent instance.
// The provider, system prompt, and dependencies must be provided.
// The Result type is inferred from the generic parameter.
func NewAgent[Deps any, Result any](
	prov provider.Provider,
	systemPrompt string,
	deps Deps,
) (*Agent[Deps, Result], error) {
	if isNilProvider(prov) {
		return nil, fmt.Errorf("provider cannot be nil")
	}

	if systemPrompt == "" {
		return nil, fmt.Errorf("system prompt cannot be empty")
	}

	resultType := reflect.TypeFor[Result]()
	resultSchema, err := schema.Generate(resultType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate result schema: %w", err)
	}

	return &Agent[Deps, Result]{
		provider:       prov,
		systemPrompt:   systemPrompt,
		tools:          tool.NewRegistry(),
		resultIsString: resultType.Kind() == reflect.String,
		resultSchema:   resultSchema,
		deps:           deps,
	}, nil
}

// isNilProvider reports whether prov is nil, including a typed nil pointer.
// Non-pointer provider implementations are never nil.
func isNilProvider(prov provider.Provider) bool {
	if prov == nil {
		return true
	}
	v := reflect.ValueOf(prov)
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// Tool registers a tool with the agent.
// The handler function must have one of these signatures:
//
//	func(ctx RunContext[Deps], args ArgsStruct) (ResultType, error)
//	func(ctx RunContext[Deps]) (ResultType, error)
//
// The parameter schema shown to the model is generated from ArgsStruct.
func (a *Agent[Deps, Result]) Tool(name, description string, handler any) error {
	handlerValue := reflect.ValueOf(handler)
	if err := a.validateToolHandler(handlerValue); err != nil {
		return fmt.Errorf("tool %s: %w", name, err)
	}

	var argsType reflect.Type
	if handlerValue.Type().NumIn() == 2 {
		argsType = handlerValue.Type().In(1)
	}
	params, err := tool.ParamsSchema(argsType)
	if err != nil {
		return fmt.Errorf("tool %s: %w", name, err)
	}

	return a.tools.RegisterHandler(name, description, params, a.wrapToolHandler(handlerValue, argsType))
}

// validateToolHandler checks the handler signature up front, so a bad handler
// fails at registration instead of panicking during a run.
func (a *Agent[Deps, Result]) validateToolHandler(v reflect.Value) error {
	if v.Kind() != reflect.Func {
		return fmt.Errorf("handler must be a function, got %s", v.Kind())
	}
	t := v.Type()
	runCtxType := reflect.TypeFor[RunContext[Deps]]()
	errorType := reflect.TypeFor[error]()

	switch {
	case t.NumIn() < 1 || t.NumIn() > 2:
		return fmt.Errorf("handler must take RunContext and optionally an args struct, got %d parameters", t.NumIn())
	case t.In(0) != runCtxType:
		return fmt.Errorf("first parameter must be %s, got %s", runCtxType, t.In(0))
	case t.NumOut() != 2:
		return fmt.Errorf("handler must return (result, error), got %d values", t.NumOut())
	case t.Out(1) != errorType:
		return fmt.Errorf("second return value must be error, got %s", t.Out(1))
	}
	return nil
}

// usageKey is the context key carrying the current run's usage to tool handlers.
type usageKey struct{}

// wrapToolHandler adapts a validated user handler to tool.Handler, injecting
// RunContext and decoding the JSON arguments into argsType.
func (a *Agent[Deps, Result]) wrapToolHandler(handler reflect.Value, argsType reflect.Type) tool.Handler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		usage, isUsage := ctx.Value(usageKey{}).(*message.Usage)
		if !isUsage {
			usage = &message.Usage{}
		}
		runCtx := RunContext[Deps]{Ctx: ctx, Deps: a.deps, Usage: usage}
		inputs := []reflect.Value{reflect.ValueOf(runCtx)}

		if argsType != nil {
			argsPtr := reflect.New(argsType)
			if len(args) > 0 {
				if err := json.Unmarshal(args, argsPtr.Interface()); err != nil {
					return nil, fmt.Errorf("failed to unmarshal arguments: %w", err)
				}
			}
			inputs = append(inputs, argsPtr.Elem())
		}

		outputs := handler.Call(inputs)
		result := outputs[0].Interface()
		if err, isErr := outputs[1].Interface().(error); isErr && err != nil {
			return result, err
		}
		return result, nil
	}
}
