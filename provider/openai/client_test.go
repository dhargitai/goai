package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/dhargitai/goai/schema"
	"github.com/dhargitai/goai/tool"
)

// completedResponse is a canned Responses API reply with a reasoning item,
// a function call and a text message.
const completedResponse = `{
	"id": "resp_1", "object": "response", "status": "completed", "model": "gpt-6.1-sol",
	"output": [
		{"type": "reasoning", "id": "rs_1", "summary": [], "encrypted_content": "enc-abc"},
		{"type": "function_call", "id": "fc_1", "call_id": "call_1", "name": "search",
		 "arguments": "{\"query\":\"go\"}", "status": "completed"},
		{"type": "message", "id": "msg_1", "role": "assistant", "status": "completed",
		 "content": [{"type": "output_text", "text": "Hello", "annotations": []}]}
	],
	"usage": {"input_tokens": 100, "input_tokens_details": {"cached_tokens": 40},
	          "output_tokens": 20, "output_tokens_details": {"reasoning_tokens": 5},
	          "total_tokens": 120}
}`

// recordingServer serves body for every request and records the decoded request bodies.
type recordingServer struct {
	*httptest.Server
	requests []map[string]any
}

func newRecordingServer(t *testing.T, contentType, body string) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rs.requests = append(rs.requests, decoded)
		w.Header().Set("Content-Type", contentType)
		if _, err := io.WriteString(w, body); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	t.Cleanup(rs.Close)
	return rs
}

func newTestClient(t *testing.T, srv *recordingServer, model string) *Client {
	t.Helper()
	client, err := New(
		WithAPIKey("test-key"),
		WithModel(model),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithReasoningEffort("low"),
	)
	require.NoError(t, err)
	return client
}

type searchArgs struct {
	Query string `json:"query" description:"Search query"`
	Limit *int   `json:"limit"`
}

type answer struct {
	Text  string `json:"text"`
	Place struct {
		City string `json:"city"`
	} `json:"place"`
}

func testRequest(t *testing.T) *provider.Request {
	t.Helper()
	params, err := tool.ParamsSchema(reflect.TypeFor[searchArgs]())
	require.NoError(t, err)
	resultSchema, err := schema.Generate(reflect.TypeFor[answer]())
	require.NoError(t, err)

	return &provider.Request{
		Messages: []message.Message{
			message.NewSystemMessage("Be brief."),
			message.NewUserMessage("Find Go news"),
			message.NewAssistantMessageWithToolCalls(
				[]message.ToolCall{{ID: "call_0", Type: "function", Function: message.FunctionCall{Name: "search", Arguments: `{"query":"go"}`}}},
				[]json.RawMessage{json.RawMessage(`{"type":"reasoning","id":"rs_0","summary":[],"encrypted_content":"enc-prev"}`)},
			),
			message.NewToolMessage("call_0", `"no results"`),
		},
		Tools:          []tool.Definition{{Name: "search", Description: "Search the web", Parameters: params}},
		Temperature:    0.3,
		MaxTokens:      200,
		ResponseFormat: &provider.ResponseFormat{Name: "answer", Schema: resultSchema},
	}
}

func TestClient_Request_SendsStatelessResponsesRequest(t *testing.T) {
	srv := newRecordingServer(t, "application/json", completedResponse)
	client := newTestClient(t, srv, "gpt-6.1-sol")

	_, err := client.Request(context.Background(), testRequest(t))
	require.NoError(t, err)
	require.Len(t, srv.requests, 1)
	body := srv.requests[0]

	assert.Equal(t, "gpt-6.1-sol", body["model"])
	assert.Equal(t, false, body["store"])
	assert.Equal(t, "Be brief.", body["instructions"])
	assert.Equal(t, float64(200), body["max_output_tokens"])
	assert.Equal(t, []any{"reasoning.encrypted_content"}, body["include"])
	assert.Equal(t, map[string]any{"effort": "low"}, body["reasoning"])
	assert.NotContains(t, body, "temperature", "reasoning models reject temperature")

	input := body["input"].([]any)
	require.Len(t, input, 4)
	assert.Equal(t, "user", input[0].(map[string]any)["role"])
	assert.Equal(t, map[string]any{"type": "reasoning", "id": "rs_0", "summary": []any{}, "encrypted_content": "enc-prev"}, input[1])
	assert.Equal(t, "function_call", input[2].(map[string]any)["type"])
	assert.Equal(t, "call_0", input[2].(map[string]any)["call_id"])
	assert.Equal(t, "function_call_output", input[3].(map[string]any)["type"])
	assert.Equal(t, "call_0", input[3].(map[string]any)["call_id"])
	assert.Equal(t, `"no results"`, input[3].(map[string]any)["output"])

	fn := body["tools"].([]any)[0].(map[string]any)
	assert.Equal(t, "function", fn["type"])
	assert.Equal(t, true, fn["strict"])
	params := fn["parameters"].(map[string]any)
	assert.Equal(t, false, params["additionalProperties"])
	assert.ElementsMatch(t, []any{"query", "limit"}, params["required"])

	format := body["text"].(map[string]any)["format"].(map[string]any)
	assert.Equal(t, "json_schema", format["type"])
	assert.Equal(t, "answer", format["name"])
	assert.Equal(t, true, format["strict"])
	place := format["schema"].(map[string]any)["properties"].(map[string]any)["place"].(map[string]any)
	assert.Contains(t, place["properties"], "city", "nested struct fields must reach the API")
}

func TestClient_Request_ModelCapabilities(t *testing.T) {
	tests := []struct {
		model           string
		wantTemperature bool
		wantReasoning   bool
	}{
		{model: "gpt-4o", wantTemperature: true},
		{model: "gpt-4o-mini", wantTemperature: true},
		{model: "gpt-4.1", wantTemperature: true},
		{model: "gpt-5-chat-latest", wantTemperature: true},
		{model: "o4-mini", wantReasoning: true},
		{model: "gpt-5.5", wantReasoning: true},
		{model: "gpt-6.1-sol", wantReasoning: true},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			srv := newRecordingServer(t, "application/json", completedResponse)
			client := newTestClient(t, srv, tt.model)

			_, err := client.Request(context.Background(), testRequest(t))
			require.NoError(t, err)
			body := srv.requests[0]

			if tt.wantTemperature {
				assert.Equal(t, 0.3, body["temperature"])
			} else {
				assert.NotContains(t, body, "temperature")
			}
			if tt.wantReasoning {
				assert.Contains(t, body, "reasoning")
				assert.Contains(t, body, "include")
			} else {
				assert.NotContains(t, body, "reasoning")
				assert.NotContains(t, body, "include")
			}
		})
	}
}

func TestClient_Request_ParsesResponse(t *testing.T) {
	srv := newRecordingServer(t, "application/json", completedResponse)
	client := newTestClient(t, srv, "gpt-6.1-sol")

	resp, err := client.Request(context.Background(), testRequest(t))
	require.NoError(t, err)

	assert.Equal(t, "Hello", resp.Content)
	assert.Equal(t, []message.ToolCall{{
		ID: "call_1", Type: "function",
		Function: message.FunctionCall{Name: "search", Arguments: `{"query":"go"}`},
	}}, resp.ToolCalls)
	require.Len(t, resp.ProviderItems, 1)
	assert.JSONEq(t, `{"type":"reasoning","id":"rs_1","summary":[],"encrypted_content":"enc-abc"}`, string(resp.ProviderItems[0]))
	assert.Equal(t, &message.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedPromptTokens: 40}, resp.Usage)
}

func TestClient_Request_ErrorResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "incomplete",
			body:    `{"id":"r","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}`,
			wantErr: "response incomplete: max_output_tokens",
		},
		{
			name:    "failed",
			body:    `{"id":"r","object":"response","status":"failed","error":{"code":"server_error","message":"boom"},"output":[]}`,
			wantErr: "response failed: server_error: boom",
		},
		{
			name: "refusal",
			body: `{"id":"r","object":"response","status":"completed","output":[{"type":"message","id":"m","role":"assistant","status":"completed",
				"content":[{"type":"refusal","refusal":"I can't help with that"}]}]}`,
			wantErr: "model refused: I can't help with that",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newRecordingServer(t, "application/json", tt.body)
			client := newTestClient(t, srv, "gpt-4o")

			_, err := client.Request(context.Background(), testRequest(t))
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestClient_Request_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	client, err := New(WithAPIKey("k"), WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	require.NoError(t, err)

	_, err = client.Request(context.Background(), &provider.Request{Messages: []message.Message{message.NewUserMessage("hi")}})
	assert.ErrorContains(t, err, "responses request failed")
}

func TestClient_InvalidInput(t *testing.T) {
	client, err := New(WithAPIKey("k"))
	require.NoError(t, err)

	_, err = client.Request(context.Background(), nil)
	assert.ErrorContains(t, err, "cannot be nil")

	badRole := &provider.Request{Messages: []message.Message{{Role: "narrator", Content: "x"}}}
	_, err = client.Request(context.Background(), badRole)
	assert.ErrorContains(t, err, `unsupported message role "narrator"`)

	badItem := &provider.Request{Messages: []message.Message{
		message.NewAssistantMessageWithToolCalls(nil, []json.RawMessage{json.RawMessage(`{broken`)}),
	}}
	_, err = client.Request(context.Background(), badItem)
	assert.ErrorContains(t, err, "invalid provider item")

	for _, err := range client.Stream(context.Background(), nil) {
		assert.ErrorContains(t, err, "cannot be nil")
	}
}

// sseBody renders events as a server-sent event stream.
func sseBody(t *testing.T, events ...string) string {
	t.Helper()
	var b strings.Builder
	for _, e := range events {
		var head struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal([]byte(e), &head))
		var compact bytes.Buffer
		require.NoError(t, json.Compact(&compact, []byte(e)))
		fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", head.Type, compact.String())
	}
	return b.String()
}

func deltaEvent(seq int, text string) string {
	return fmt.Sprintf(`{"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":%q,"sequence_number":%d}`, text, seq)
}

func TestClient_Stream_YieldsDeltasThenFinalResponse(t *testing.T) {
	body := sseBody(t,
		deltaEvent(1, "Hel"),
		deltaEvent(2, "lo"),
		`{"type":"response.completed","sequence_number":3,"response":`+completedResponse+`}`,
	)
	srv := newRecordingServer(t, "text/event-stream", body)
	client := newTestClient(t, srv, "gpt-4o")

	var deltas []string
	var final *provider.Response
	for chunk, err := range client.Stream(context.Background(), testRequest(t)) {
		require.NoError(t, err)
		if chunk.Response != nil {
			final = chunk.Response
			continue
		}
		deltas = append(deltas, chunk.Content)
	}

	assert.Equal(t, []string{"Hel", "lo"}, deltas)
	require.NotNil(t, final)
	assert.Equal(t, "Hello", final.Content)
	assert.Len(t, final.ToolCalls, 1)
	assert.Equal(t, 120, final.Usage.TotalTokens)
	assert.Equal(t, true, srv.requests[0]["stream"])
}

func TestClient_Stream_EarlyBreakStopsIteration(t *testing.T) {
	body := sseBody(t,
		deltaEvent(1, "a"),
		deltaEvent(2, "b"),
		`{"type":"response.completed","sequence_number":3,"response":`+completedResponse+`}`,
	)
	srv := newRecordingServer(t, "text/event-stream", body)
	client := newTestClient(t, srv, "gpt-4o")

	count := 0
	for _, err := range client.Stream(context.Background(), testRequest(t)) {
		require.NoError(t, err)
		count++
		break
	}
	assert.Equal(t, 1, count)
}

func TestClient_Stream_Errors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "error event",
			body:    sseBody(t, `{"type":"error","code":"rate_limit","message":"slow down","sequence_number":1}`),
			wantErr: "stream error rate_limit: slow down",
		},
		{
			name:    "no terminal event",
			body:    sseBody(t, deltaEvent(1, "a")),
			wantErr: "stream ended without a final response",
		},
		{
			name: "incomplete",
			body: sseBody(t, `{"type":"response.incomplete","sequence_number":1,"response":`+
				`{"id":"r","object":"response","status":"incomplete","incomplete_details":{"reason":"content_filter"},"output":[]}}`),
			wantErr: "response incomplete: content_filter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newRecordingServer(t, "text/event-stream", tt.body)
			client := newTestClient(t, srv, "gpt-4o")

			var lastErr error
			for _, err := range client.Stream(context.Background(), testRequest(t)) {
				if err != nil {
					lastErr = err
				}
			}
			assert.ErrorContains(t, lastErr, tt.wantErr)
		})
	}
}

func TestClient_EstimateCost(t *testing.T) {
	client, err := New(WithAPIKey("k"), WithModel("gpt-4o-2024-08-06"))
	require.NoError(t, err)

	usage := &message.Usage{PromptTokens: 1_000_000, CompletionTokens: 1_000_000}
	assert.InDelta(t, 12.50, client.EstimateCost(usage), 1e-9)
}

func TestNew_WithEnvDefaults(t *testing.T) {
	tests := []struct {
		name      string
		envModel  string
		envAPIKey string
		wantModel string
		wantErr   bool
	}{
		{name: "env vars set", envModel: "gpt-4o", envAPIKey: "sk-env-key", wantModel: "gpt-4o"},
		{name: "model not set uses default", envAPIKey: "sk-env-key", wantModel: DefaultModel},
		{name: "missing API key", envModel: "gpt-4o", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OPENAI_MODEL", tt.envModel)
			t.Setenv("OPENAI_API_KEY", tt.envAPIKey)

			client, err := New()

			if tt.wantErr {
				assert.ErrorContains(t, err, "API key required")
				assert.Nil(t, client)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, client.Name())
		})
	}
}

func TestNew_OptionsOverrideEnv(t *testing.T) {
	t.Setenv("OPENAI_MODEL", "gpt-4o")
	t.Setenv("OPENAI_API_KEY", "sk-env-key")

	tests := []struct {
		name      string
		opts      []Option
		wantModel string
	}{
		{name: "WithModel overrides env", opts: []Option{WithModel("gpt-4.1")}, wantModel: "gpt-4.1"},
		{name: "WithAPIKey keeps env model", opts: []Option{WithAPIKey("sk-opt-key")}, wantModel: "gpt-4o"},
		{name: "both options", opts: []Option{WithModel("gpt-5.5"), WithAPIKey("sk-opt-key")}, wantModel: "gpt-5.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := New(tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, client.Name())
		})
	}
}

func TestNew_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	client, err := New(WithModel("gpt-4o"))

	assert.Nil(t, client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OPENAI_API_KEY")
	assert.Contains(t, err.Error(), "WithAPIKey")
}
