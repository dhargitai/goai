package mock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
)

func TestNew(t *testing.T) {
	p := New()
	assert.Equal(t, "mock-model", p.Name())
	assert.NotNil(t, p.Response)
}

func TestProvider_Request(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*Provider)
		want    string
		wantErr bool
	}{
		{
			name:  "configured response",
			setup: func(p *Provider) { p.WithResponse(&provider.Response{Content: "Test response"}) },
			want:  "Test response",
		},
		{
			name:    "error",
			setup:   func(p *Provider) { p.WithError(assert.AnError) },
			wantErr: true,
		},
		{
			name:    "no response configured",
			setup:   func(p *Provider) { p.Response = nil },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()
			tt.setup(p)

			resp, err := p.Request(context.Background(), &provider.Request{})
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, resp.Content)
		})
	}
}

func TestProvider_ResponsesQueue(t *testing.T) {
	p := New().
		WithResponses(&provider.Response{Content: "first"}, &provider.Response{Content: "second"}).
		WithResponse(&provider.Response{Content: "fallback"})

	var got []string
	for range 4 {
		resp, err := p.Request(context.Background(), &provider.Request{})
		require.NoError(t, err)
		got = append(got, resp.Content)
	}

	assert.Equal(t, []string{"first", "second", "fallback", "fallback"}, got)
	assert.Len(t, p.Requests, 4)
}

func TestProvider_RequestCallback(t *testing.T) {
	var calls int
	p := New().WithRequestCallback(func(ctx context.Context, req *provider.Request) { calls++ })

	_, err := p.Request(context.Background(), &provider.Request{})
	require.NoError(t, err)
	for _, err := range p.Stream(context.Background(), &provider.Request{}) {
		require.NoError(t, err)
	}

	assert.Equal(t, 2, calls)
}

func TestProvider_Stream(t *testing.T) {
	p := New()
	p.SetStreamResponse("Hello, world!")

	var pieces []string
	var final *provider.Response
	for chunk, err := range p.Stream(context.Background(), &provider.Request{}) {
		require.NoError(t, err)
		if chunk.Response != nil {
			final = chunk.Response
			continue
		}
		pieces = append(pieces, chunk.Content)
	}

	assert.Equal(t, []string{"Hello", ", wor", "ld!"}, pieces)
	require.NotNil(t, final)
	assert.Equal(t, "Hello, world!", final.Content)
}

func TestProvider_Stream_Error(t *testing.T) {
	p := New().WithError(assert.AnError)

	var errs []error
	for _, err := range p.Stream(context.Background(), &provider.Request{}) {
		errs = append(errs, err)
	}
	assert.Equal(t, []error{assert.AnError}, errs)
}

func TestProvider_Stream_CancelledContext(t *testing.T) {
	p := New()
	p.SetStreamResponse("some content")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var lastErr error
	for _, err := range p.Stream(ctx, &provider.Request{}) {
		lastErr = err
	}
	assert.ErrorIs(t, lastErr, context.Canceled)
}

func TestProvider_Stream_EarlyBreak(t *testing.T) {
	p := New()
	p.SetStreamResponse("abcdefghijklmnop")

	count := 0
	for _, err := range p.Stream(context.Background(), &provider.Request{}) {
		require.NoError(t, err)
		count++
		break
	}
	assert.Equal(t, 1, count)
}

func TestProvider_SetStreamToolCall(t *testing.T) {
	p := New()
	p.SetStreamToolCall("search", `{"query":"go"}`)

	resp, err := p.Request(context.Background(), &provider.Request{})
	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	assert.Equal(t, message.FunctionCall{Name: "search", Arguments: `{"query":"go"}`}, resp.ToolCalls[0].Function)
}

func TestProvider_WithToolCalls(t *testing.T) {
	calls := []message.ToolCall{{ID: "c1", Function: message.FunctionCall{Name: "t"}}}
	p := (&Provider{}).WithToolCalls(calls)
	assert.Equal(t, calls, p.Response.ToolCalls)
}
