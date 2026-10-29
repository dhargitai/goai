package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
)

// errStopped reports that a streaming consumer stopped iterating.
var errStopped = errors.New("stream consumer stopped")

// runState is the conversation and usage of a single agent run.
type runState struct {
	messages []message.Message
	usage    *message.Usage
}

// Run executes the agent with the given user prompt.
// It handles message building, provider requests, tool execution loops,
// and structured output parsing.
func (a *Agent[Deps, Result]) Run(ctx context.Context, userPrompt string) (*RunResult[Result], error) {
	state, err := a.start(userPrompt)
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, usageKey{}, state.usage)

	for iteration := 1; iteration <= maxIterations; iteration++ {
		resp, err := a.provider.Request(ctx, a.buildRequest(state))
		if err != nil {
			return nil, fmt.Errorf("provider request failed (iteration %d): %w", iteration, err)
		}

		done, err := a.handleResponse(ctx, state, resp, nil)
		if err != nil || done {
			return a.finish(state, resp, err)
		}
	}

	return nil, fmt.Errorf("max iterations (%d) reached without completion", maxIterations)
}

// start validates the prompt and creates the initial run state.
func (a *Agent[Deps, Result]) start(userPrompt string) (*runState, error) {
	if userPrompt == "" {
		return nil, fmt.Errorf("user prompt cannot be empty")
	}
	return &runState{
		messages: []message.Message{
			message.NewSystemMessage(a.systemPrompt),
			message.NewUserMessage(userPrompt),
		},
		usage: &message.Usage{},
	}, nil
}

// buildRequest creates the provider request for the current conversation.
func (a *Agent[Deps, Result]) buildRequest(state *runState) *provider.Request {
	req := &provider.Request{
		Messages: state.messages,
		Tools:    a.tools.GetDefinitions(),
	}
	if !a.resultIsString {
		req.ResponseFormat = &provider.ResponseFormat{
			Name:   "result",
			Schema: a.resultSchema,
		}
	}
	return req
}

// handleResponse records a model response in the run state. If the model
// asked for tools, it executes them and returns false so the loop continues;
// otherwise it returns true. onTool, when non-nil, is called before each tool
// runs; returning false stops the run with errStopped.
func (a *Agent[Deps, Result]) handleResponse(
	ctx context.Context,
	state *runState,
	resp *provider.Response,
	onTool func(name string) bool,
) (bool, error) {
	state.usage.Add(resp.Usage)

	if len(resp.ToolCalls) == 0 {
		state.messages = append(state.messages, message.NewAssistantMessage(resp.Content))
		return true, nil
	}

	state.messages = append(state.messages,
		message.NewAssistantMessageWithToolCalls(resp.ToolCalls, resp.ProviderItems))

	for _, call := range resp.ToolCalls {
		if onTool != nil && !onTool(call.Function.Name) {
			return false, errStopped
		}
		state.messages = append(state.messages,
			message.NewToolMessage(call.ID, a.executeToolCall(ctx, call)))
	}
	return false, nil
}

// executeToolCall runs a tool and renders its result (or error) for the model.
// Tool errors are reported to the model rather than failing the run, so the
// model can recover, e.g. by retrying with corrected arguments.
func (a *Agent[Deps, Result]) executeToolCall(ctx context.Context, call message.ToolCall) string {
	result, err := a.tools.Execute(ctx, call.Function.Name, json.RawMessage(call.Function.Arguments))
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("Error marshaling result: %v", err)
	}
	return string(resultJSON)
}

// finish parses the final response into a RunResult.
func (a *Agent[Deps, Result]) finish(state *runState, resp *provider.Response, err error) (*RunResult[Result], error) {
	if err != nil {
		return nil, err
	}

	var result Result
	if resp.Content != "" {
		if a.resultIsString {
			reflect.ValueOf(&result).Elem().SetString(resp.Content)
		} else if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
			return nil, fmt.Errorf("failed to parse structured output: %w", err)
		}
	}

	return &RunResult[Result]{
		Data:     result,
		Messages: state.messages,
		Usage:    state.usage,
		Cost:     a.estimateCost(state.usage),
	}, nil
}

// estimateCost asks the provider for a cost estimate if it supports one.
func (a *Agent[Deps, Result]) estimateCost(usage *message.Usage) float64 {
	estimator, ok := a.provider.(provider.CostEstimator)
	if !ok {
		return 0
	}
	return estimator.EstimateCost(usage)
}
