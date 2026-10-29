package agent

import (
	"context"
	"iter"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/dhargitai/goai/provider/mock"
)

// collect drains a stream, returning the text, tool names, final result and last error.
func collect[R any](t *testing.T, ag *Agent[SimpleDeps, R], ctx context.Context, prompt string) (string, []string, *RunResult[R], error) {
	t.Helper()
	var text strings.Builder
	var tools []string
	var result *RunResult[R]
	for ev, err := range ag.RunStream(ctx, prompt) {
		if err != nil {
			return text.String(), tools, result, err
		}
		text.WriteString(ev.Content)
		if ev.ToolName != "" {
			tools = append(tools, ev.ToolName)
		}
		if ev.Result != nil {
			result = ev.Result
		}
	}
	return text.String(), tools, result, nil
}

func TestAgent_RunStream_Text(t *testing.T) {
	prov := mock.New()
	prov.SetStreamResponse("Hello, streaming world!")
	ag, err := NewAgent[SimpleDeps, string](prov, "Test", SimpleDeps{})
	require.NoError(t, err)

	text, tools, result, err := collect(t, ag, context.Background(), "Say hello")

	require.NoError(t, err)
	assert.Equal(t, "Hello, streaming world!", text)
	assert.Empty(t, tools)
	require.NotNil(t, result)
	assert.Equal(t, "Hello, streaming world!", result.Data)
	assert.Equal(t, 15, result.Usage.TotalTokens)
}

func TestAgent_RunStream_StructuredOutputOnlyYieldsResult(t *testing.T) {
	prov := mock.New()
	prov.SetStreamResponse(`{"answer": "Test answer"}`)
	ag, err := NewAgent[SimpleDeps, SimpleResult](prov, "Test", SimpleDeps{})
	require.NoError(t, err)

	text, _, result, err := collect(t, ag, context.Background(), "Generate")

	require.NoError(t, err)
	assert.Empty(t, text, "raw JSON fragments are not streamed as text")
	require.NotNil(t, result)
	assert.Equal(t, "Test answer", result.Data.Answer)
}

func TestAgent_RunStream_WithToolCall(t *testing.T) {
	prov := mock.New().WithResponses(
		&provider.Response{ToolCalls: []message.ToolCall{searchCall("c1")}, Usage: &message.Usage{TotalTokens: 10}},
		&provider.Response{Content: "Found it", Usage: &message.Usage{TotalTokens: 20}},
	)
	ag, err := NewAgent[SimpleDeps, string](prov, "Test", SimpleDeps{})
	require.NoError(t, err)
	var executed int
	require.NoError(t, ag.Tool("search", "Search", func(ctx RunContext[SimpleDeps], args searchArgs) (string, error) {
		executed++
		return "result for " + args.Query, nil
	}))

	text, tools, result, err := collect(t, ag, context.Background(), "Find")

	require.NoError(t, err)
	assert.Equal(t, []string{"search"}, tools)
	assert.Equal(t, 1, executed)
	assert.Equal(t, "Found it", text)
	require.NotNil(t, result)
	assert.Equal(t, 30, result.Usage.TotalTokens)
	assert.Len(t, result.Messages, 5)
	assert.Equal(t, `"result for go"`, result.Messages[3].Content)
}

// Regression: the channel-based stream left its goroutine blocked forever when
// the consumer stopped reading. synctest.Test fails if any goroutine started
// in the bubble is still blocked when the test function returns.
func TestAgent_RunStream_EarlyBreakLeavesNothingRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prov := mock.New()
		prov.SetStreamResponse(strings.Repeat("chunk", 50))
		ag, err := NewAgent[SimpleDeps, string](prov, "Test", SimpleDeps{})
		require.NoError(t, err)

		events := 0
		for ev, err := range ag.RunStream(context.Background(), "Talk") {
			require.NoError(t, err)
			require.NotEmpty(t, ev.Content)
			events++
			break
		}

		synctest.Wait()
		assert.Equal(t, 1, events)
		assert.Len(t, prov.Requests, 1)
	})
}

func TestAgent_RunStream_BreakOnToolEventSkipsTool(t *testing.T) {
	prov := mock.New().WithResponses(
		&provider.Response{ToolCalls: []message.ToolCall{searchCall("c1")}},
		&provider.Response{Content: "unreachable"},
	)
	ag, err := NewAgent[SimpleDeps, string](prov, "Test", SimpleDeps{})
	require.NoError(t, err)
	executed := false
	require.NoError(t, ag.Tool("search", "Search", func(ctx RunContext[SimpleDeps], args searchArgs) (string, error) {
		executed = true
		return "", nil
	}))

	for ev, err := range ag.RunStream(context.Background(), "Find") {
		require.NoError(t, err)
		if ev.ToolName != "" {
			break
		}
	}

	assert.False(t, executed)
	assert.Len(t, prov.Requests, 1)
}

func TestAgent_RunStream_Errors(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		prov    *mock.Provider
		ctx     context.Context
		prompt  string
		wantErr string
	}{
		{name: "empty prompt", prov: mock.New(), ctx: context.Background(), prompt: "", wantErr: "user prompt cannot be empty"},
		{name: "provider error", prov: mock.New().WithError(assert.AnError), ctx: context.Background(), prompt: "hi", wantErr: "provider stream failed (iteration 1)"},
		{name: "cancelled context", prov: mock.New(), ctx: cancelled, prompt: "hi", wantErr: "context canceled"},
		{
			name:    "max iterations",
			prov:    mock.New().WithResponse(&provider.Response{ToolCalls: []message.ToolCall{searchCall("loop")}}),
			ctx:     context.Background(),
			prompt:  "hi",
			wantErr: "max iterations (10) reached",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ag, err := NewAgent[SimpleDeps, string](tt.prov, "Test", SimpleDeps{})
			require.NoError(t, err)
			require.NoError(t, ag.Tool("search", "Search", func(ctx RunContext[SimpleDeps], args searchArgs) (string, error) {
				return "", nil
			}))

			_, _, result, err := collect(t, ag, tt.ctx, tt.prompt)

			assert.ErrorContains(t, err, tt.wantErr)
			assert.Nil(t, result)
		})
	}
}

// endlessProvider streams content without ever sending a final response.
type endlessProvider struct{ *mock.Provider }

func (endlessProvider) Stream(ctx context.Context, req *provider.Request) iter.Seq2[*provider.Chunk, error] {
	return func(yield func(*provider.Chunk, error) bool) {
		yield(&provider.Chunk{Content: "partial"}, nil)
	}
}

func TestAgent_RunStream_MissingFinalResponse(t *testing.T) {
	ag, err := NewAgent[SimpleDeps, string](endlessProvider{mock.New()}, "Test", SimpleDeps{})
	require.NoError(t, err)

	_, _, _, err = collect(t, ag, context.Background(), "hi")
	assert.ErrorContains(t, err, "ended without a final response")
}
