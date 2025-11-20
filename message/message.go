// Package message provides types for representing messages exchanged
// between the user, assistant, and tools in AI agent conversations.
package message

// Message represents a single message in a conversation.
// It can be from the system, user, assistant, or a tool response.
type Message struct {
	// Role identifies the message sender: "system", "user", "assistant", or "tool"
	Role string `json:"role"`

	// Content is the text content of the message
	Content string `json:"content,omitempty"`

	// ToolCalls contains function calls requested by the assistant (only for assistant messages)
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`

	// ToolCallID is the ID of the tool call this message is responding to (only for tool messages)
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ToolCall represents a request to execute a function/tool.
// Used by the LLM to indicate it wants to call a registered tool.
type ToolCall struct {
	// ID is a unique identifier for this tool call
	ID string `json:"id"`

	// Type is always "function" for function calls
	Type string `json:"type"`

	// Function contains the function name and arguments
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the details of a function/tool to be called.
type FunctionCall struct {
	// Name is the name of the function to call
	Name string `json:"name"`

	// Arguments is a JSON string containing the function arguments
	Arguments string `json:"arguments"`
}

// Usage tracks token consumption for API requests.
type Usage struct {
	// PromptTokens is the number of tokens in the prompt/input
	PromptTokens int `json:"prompt_tokens"`

	// CompletionTokens is the number of tokens in the completion/output
	CompletionTokens int `json:"completion_tokens"`

	// TotalTokens is the sum of prompt and completion tokens
	TotalTokens int `json:"total_tokens"`
}

// Role constants for message types
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// NewSystemMessage creates a new system message.
// System messages set the behavior and context for the assistant.
func NewSystemMessage(content string) Message {
	return Message{
		Role:    RoleSystem,
		Content: content,
	}
}

// NewUserMessage creates a new user message.
// User messages contain the user's input or question.
func NewUserMessage(content string) Message {
	return Message{
		Role:    RoleUser,
		Content: content,
	}
}

// NewAssistantMessage creates a new assistant message.
// Assistant messages contain the AI's response.
func NewAssistantMessage(content string) Message {
	return Message{
		Role:    RoleAssistant,
		Content: content,
	}
}

// NewAssistantMessageWithToolCalls creates an assistant message with tool calls.
// Used when the assistant wants to execute one or more tools.
func NewAssistantMessageWithToolCalls(toolCalls []ToolCall) Message {
	return Message{
		Role:      RoleAssistant,
		ToolCalls: toolCalls,
	}
}

// NewToolMessage creates a new tool response message.
// Tool messages contain the result of executing a tool.
func NewToolMessage(toolCallID, content string) Message {
	return Message{
		Role:       RoleTool,
		Content:    content,
		ToolCallID: toolCallID,
	}
}
