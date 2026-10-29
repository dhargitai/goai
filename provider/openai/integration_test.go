//go:build integration

package openai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
)

// Live tests against the OpenAI API. They cost a small amount of money.
// Run with: OPENAI_API_KEY=sk-... go test -tags=integration ./provider/openai/...
// Set OPENAI_MODEL to test a model other than DefaultModel.
func newLiveClient(t *testing.T) *Client {
	t.Helper()
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not set")
	}
	client, err := New()
	require.NoError(t, err)
	return client
}

func TestIntegration_Request(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	resp, err := client.Request(ctx, &provider.Request{
		Messages: []message.Message{message.NewUserMessage("Say 'Hello, World!' and nothing else")},
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Content, "Hello")
	assert.Positive(t, resp.Usage.TotalTokens)
}

func TestIntegration_Stream(t *testing.T) {
	client := newLiveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var deltas int
	var final *provider.Response
	for chunk, err := range client.Stream(ctx, &provider.Request{
		Messages: []message.Message{message.NewUserMessage("Count from 1 to 5")},
	}) {
		require.NoError(t, err)
		if chunk.Response != nil {
			final = chunk.Response
		} else {
			deltas++
		}
	}
	require.NotNil(t, final)
	assert.Positive(t, deltas)
	assert.NotEmpty(t, final.Content)
}
