package message

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSystemMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Message
	}{
		{
			name:    "simple system message",
			content: "You are a helpful assistant",
			want: Message{
				Role:    RoleSystem,
				Content: "You are a helpful assistant",
			},
		},
		{
			name:    "empty content",
			content: "",
			want: Message{
				Role:    RoleSystem,
				Content: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewSystemMessage(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewUserMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Message
	}{
		{
			name:    "simple user message",
			content: "What is the weather?",
			want: Message{
				Role:    RoleUser,
				Content: "What is the weather?",
			},
		},
		{
			name:    "empty content",
			content: "",
			want: Message{
				Role:    RoleUser,
				Content: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewUserMessage(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewAssistantMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Message
	}{
		{
			name:    "simple assistant message",
			content: "The weather is sunny and 72°F",
			want: Message{
				Role:    RoleAssistant,
				Content: "The weather is sunny and 72°F",
			},
		},
		{
			name:    "empty content",
			content: "",
			want: Message{
				Role:    RoleAssistant,
				Content: "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAssistantMessage(tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewAssistantMessageWithToolCalls(t *testing.T) {
	tests := []struct {
		name      string
		toolCalls []ToolCall
		want      Message
	}{
		{
			name: "single tool call",
			toolCalls: []ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"San Francisco"}`,
					},
				},
			},
			want: Message{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"San Francisco"}`,
						},
					},
				},
			},
		},
		{
			name: "multiple tool calls",
			toolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"New York"}`,
					},
				},
				{
					ID:   "call_2",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_forecast",
						Arguments: `{"days":5}`,
					},
				},
			},
			want: Message{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"New York"}`,
						},
					},
					{
						ID:   "call_2",
						Type: "function",
						Function: FunctionCall{
							Name:      "get_forecast",
							Arguments: `{"days":5}`,
						},
					},
				},
			},
		},
		{
			name:      "empty tool calls",
			toolCalls: []ToolCall{},
			want: Message{
				Role:      RoleAssistant,
				ToolCalls: []ToolCall{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAssistantMessageWithToolCalls(tt.toolCalls, nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewToolMessage(t *testing.T) {
	tests := []struct {
		name       string
		toolCallID string
		content    string
		want       Message
	}{
		{
			name:       "simple tool response",
			toolCallID: "call_123",
			content:    `{"temperature":72,"condition":"sunny"}`,
			want: Message{
				Role:       RoleTool,
				Content:    `{"temperature":72,"condition":"sunny"}`,
				ToolCallID: "call_123",
			},
		},
		{
			name:       "empty content",
			toolCallID: "call_456",
			content:    "",
			want: Message{
				Role:       RoleTool,
				Content:    "",
				ToolCallID: "call_456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewToolMessage(tt.toolCallID, tt.content)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMessage_JSON(t *testing.T) {
	tests := []struct {
		name    string
		message Message
		wantErr bool
	}{
		{
			name:    "system message",
			message: NewSystemMessage("You are helpful"),
			wantErr: false,
		},
		{
			name:    "user message",
			message: NewUserMessage("Hello"),
			wantErr: false,
		},
		{
			name:    "assistant message",
			message: NewAssistantMessage("Hi there!"),
			wantErr: false,
		},
		{
			name: "assistant with tool calls",
			message: NewAssistantMessageWithToolCalls([]ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "test",
						Arguments: `{"arg":"value"}`,
					},
				},
			}, nil),
			wantErr: false,
		},
		{
			name:    "tool message",
			message: NewToolMessage("call_1", "result"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.message)
			require.NoError(t, err)

			// Unmarshal back
			var got Message
			err = json.Unmarshal(data, &got)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.message, got)
		})
	}
}

func TestUsage(t *testing.T) {
	tests := []struct {
		name  string
		usage Usage
	}{
		{
			name: "basic usage",
			usage: Usage{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		},
		{
			name: "zero usage",
			usage: Usage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.usage)
			require.NoError(t, err)

			// Unmarshal back
			var got Usage
			err = json.Unmarshal(data, &got)
			require.NoError(t, err)
			assert.Equal(t, tt.usage, got)
		})
	}
}

func TestRoleConstants(t *testing.T) {
	assert.Equal(t, "system", RoleSystem)
	assert.Equal(t, "user", RoleUser)
	assert.Equal(t, "assistant", RoleAssistant)
	assert.Equal(t, "tool", RoleTool)
}

func TestNewAssistantMessageWithToolCalls_ProviderItems(t *testing.T) {
	items := []json.RawMessage{json.RawMessage(`{"type":"reasoning"}`)}
	got := NewAssistantMessageWithToolCalls(nil, items)
	assert.Equal(t, items, got.ProviderItems)
}

func TestUsage_Add(t *testing.T) {
	u := &Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, CachedPromptTokens: 1}
	u.Add(&Usage{PromptTokens: 10, CompletionTokens: 20, TotalTokens: 30, CachedPromptTokens: 5})
	u.Add(nil)
	assert.Equal(t, &Usage{PromptTokens: 11, CompletionTokens: 22, TotalTokens: 33, CachedPromptTokens: 6}, u)
}
