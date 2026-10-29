package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/dhargitai/goai/provider/mock"
)

type searchArgs struct {
	Query string `json:"query" description:"What to search for"`
}

func searchCall(id string) message.ToolCall {
	return message.ToolCall{
		ID:       id,
		Type:     "function",
		Function: message.FunctionCall{Name: "search", Arguments: `{"query":"go"}`},
	}
}

// Regression: agent tools used to be registered through a json.RawMessage
// wrapper, so the model was sent a byte-array schema instead of the args struct.
func TestAgent_Tool_SendsArgsSchema(t *testing.T) {
	prov := mock.New().WithResponse(&provider.Response{Content: `{"answer":"ok"}`})
	ag, err := NewAgent[SimpleDeps, SimpleResult](prov, "Test", SimpleDeps{})
	require.NoError(t, err)
	require.NoError(t, ag.Tool("search", "Search", func(ctx RunContext[SimpleDeps], args searchArgs) (string, error) {
		return args.Query, nil
	}))

	_, err = ag.Run(context.Background(), "hi")
	require.NoError(t, err)

	tools := prov.Requests[0].Tools
	require.Len(t, tools, 1)
	params := tools[0].Parameters
	assert.Equal(t, "object", params.Type)
	require.Contains(t, params.Properties, "query")
	assert.Equal(t, "string", params.Properties["query"].Type)
	assert.Equal(t, "What to search for", params.Properties["query"].Description)
}

func TestAgent_Tool_RejectsBadSignatures(t *testing.T) {
	tests := []struct {
		name    string
		handler any
		wantErr string
	}{
		{name: "not a function", handler: "nope", wantErr: "must be a function"},
		{name: "no parameters", handler: func() (string, error) { return "", nil }, wantErr: "must take RunContext"},
		{name: "wrong context type", handler: func(ctx context.Context) (string, error) { return "", nil }, wantErr: "first parameter must be"},
		{name: "single return", handler: func(ctx RunContext[SimpleDeps]) error { return nil }, wantErr: "must return (result, error)"},
		{name: "second return not error", handler: func(ctx RunContext[SimpleDeps]) (string, string) { return "", "" }, wantErr: "second return value must be error"},
		{name: "non-struct args", handler: func(ctx RunContext[SimpleDeps], q string) (string, error) { return q, nil }, wantErr: "must be a struct"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag, err := NewAgent[SimpleDeps, SimpleResult](mock.New(), "Test", SimpleDeps{})
			require.NoError(t, err)

			err = ag.Tool("bad", "Bad tool", tt.handler)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// Regression: tool handlers used to receive a fresh, empty Usage.
func TestAgent_Tool_SeesRunUsage(t *testing.T) {
	prov := mock.New().WithResponses(
		&provider.Response{ToolCalls: []message.ToolCall{searchCall("c1")}, Usage: &message.Usage{TotalTokens: 15}},
		&provider.Response{Content: `{"answer":"done"}`, Usage: &message.Usage{TotalTokens: 35}},
	)
	ag, err := NewAgent[SimpleDeps, SimpleResult](prov, "Test", SimpleDeps{})
	require.NoError(t, err)

	var seen int
	require.NoError(t, ag.Tool("search", "Search", func(ctx RunContext[SimpleDeps], args searchArgs) (string, error) {
		seen = ctx.Usage.TotalTokens
		return "result", nil
	}))

	result, err := ag.Run(context.Background(), "hi")
	require.NoError(t, err)
	assert.Equal(t, 15, seen)
	assert.Equal(t, 50, result.Usage.TotalTokens)
}

func TestAgent_Run_ReplaysProviderItems(t *testing.T) {
	reasoning := json.RawMessage(`{"type":"reasoning","encrypted_content":"secret"}`)
	prov := mock.New().WithResponses(
		&provider.Response{ToolCalls: []message.ToolCall{searchCall("c1")}, ProviderItems: []json.RawMessage{reasoning}},
		&provider.Response{Content: `{"answer":"done"}`},
	)
	ag, err := NewAgent[SimpleDeps, SimpleResult](prov, "Test", SimpleDeps{})
	require.NoError(t, err)
	require.NoError(t, ag.Tool("search", "Search", func(ctx RunContext[SimpleDeps], args searchArgs) (string, error) {
		return "result", nil
	}))

	_, err = ag.Run(context.Background(), "hi")
	require.NoError(t, err)

	require.Len(t, prov.Requests, 2)
	second := prov.Requests[1].Messages
	require.Len(t, second, 4) // system, user, assistant tool call, tool result
	assert.Equal(t, []json.RawMessage{reasoning}, second[2].ProviderItems)
	assert.Equal(t, "c1", second[3].ToolCallID)
}

type pricedProvider struct {
	*mock.Provider
}

func (pricedProvider) EstimateCost(usage *message.Usage) float64 {
	return float64(usage.TotalTokens) / 100
}

func TestAgent_Run_UsesProviderCostEstimate(t *testing.T) {
	prov := pricedProvider{mock.New().WithResponse(&provider.Response{
		Content: `{"answer":"x"}`,
		Usage:   &message.Usage{TotalTokens: 250},
	})}
	ag, err := NewAgent[SimpleDeps, SimpleResult](prov, "Test", SimpleDeps{})
	require.NoError(t, err)

	result, err := ag.Run(context.Background(), "hi")
	require.NoError(t, err)
	assert.InDelta(t, 2.5, result.Cost, 1e-9)
}

func TestAgent_Run_InvalidStructuredOutput(t *testing.T) {
	prov := mock.New().WithResponse(&provider.Response{Content: "not json"})
	ag, err := NewAgent[SimpleDeps, SimpleResult](prov, "Test", SimpleDeps{})
	require.NoError(t, err)

	_, err = ag.Run(context.Background(), "hi")
	assert.ErrorContains(t, err, "failed to parse structured output")
}

func TestAgent_Run_ProviderError(t *testing.T) {
	ag, err := NewAgent[SimpleDeps, SimpleResult](mock.New().WithError(assert.AnError), "Test", SimpleDeps{})
	require.NoError(t, err)

	_, err = ag.Run(context.Background(), "hi")
	assert.ErrorIs(t, err, assert.AnError)
	assert.True(t, strings.Contains(err.Error(), "iteration 1"))
}
