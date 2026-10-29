package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/dhargitai/goai/message"
	"github.com/dhargitai/goai/provider"
	"github.com/dhargitai/goai/schema"
	"github.com/dhargitai/goai/tool"
)

// buildParams converts a provider request into stateless Responses API params:
// the full conversation is sent every time and nothing is stored server-side.
func (c *Client) buildParams(req *provider.Request) (responses.ResponseNewParams, error) {
	instructions, input, err := convertMessages(req.Messages)
	if err != nil {
		return responses.ResponseNewParams{}, fmt.Errorf("failed to convert messages: %w", err)
	}

	params := responses.ResponseNewParams{
		Model: c.model,
		Store: openai.Bool(false),
		Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input},
	}
	if instructions != "" {
		params.Instructions = openai.String(instructions)
	}
	if req.MaxTokens > 0 {
		params.MaxOutputTokens = openai.Int(int64(req.MaxTokens))
	}

	if isReasoningModel(c.model) {
		// Without server-side storage, reasoning must travel with the request.
		params.Include = []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent}
		if c.reasoningEffort != "" {
			params.Reasoning = shared.ReasoningParam{Effort: shared.ReasoningEffort(c.reasoningEffort)}
		}
	} else if req.Temperature > 0 {
		params.Temperature = openai.Float(req.Temperature)
	}

	if params.Tools, err = convertTools(req.Tools); err != nil {
		return responses.ResponseNewParams{}, err
	}
	if req.ResponseFormat != nil {
		if params.Text, err = convertResponseFormat(req.ResponseFormat); err != nil {
			return responses.ResponseNewParams{}, err
		}
	}
	return params, nil
}

// convertMessages maps the conversation to Responses input items. System
// messages become the instructions.
func convertMessages(msgs []message.Message) (string, responses.ResponseInputParam, error) {
	var instructions []string
	input := make(responses.ResponseInputParam, 0, len(msgs))

	for _, msg := range msgs {
		switch msg.Role {
		case message.RoleSystem:
			instructions = append(instructions, msg.Content)
		case message.RoleUser:
			input = append(input, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleUser))
		case message.RoleAssistant:
			items, err := assistantItems(msg)
			if err != nil {
				return "", nil, err
			}
			input = append(input, items...)
		case message.RoleTool:
			output := responses.ResponseInputItemParamOfFunctionCallOutput(msg.Content)
			output.OfFunctionCallOutput.CallID = openai.String(msg.ToolCallID)
			input = append(input, output)
		default:
			return "", nil, fmt.Errorf("unsupported message role %q", msg.Role)
		}
	}
	return strings.Join(instructions, "\n\n"), input, nil
}

// assistantItems maps an assistant message to input items: replayed provider
// items (reasoning) first, then the text, then the function calls it made.
func assistantItems(msg message.Message) ([]responses.ResponseInputItemUnionParam, error) {
	items := make([]responses.ResponseInputItemUnionParam, 0, len(msg.ProviderItems)+len(msg.ToolCalls)+1)
	for _, raw := range msg.ProviderItems {
		if !json.Valid(raw) {
			return nil, fmt.Errorf("invalid provider item: %s", raw)
		}
		var item responses.ResponseInputItemUnionParam
		param.SetJSON(raw, &item)
		items = append(items, item)
	}
	if msg.Content != "" {
		items = append(items, responses.ResponseInputItemParamOfMessage(msg.Content, responses.EasyInputMessageRoleAssistant))
	}
	for _, tc := range msg.ToolCalls {
		items = append(items, responses.ResponseInputItemParamOfFunctionCall(tc.Function.Arguments, tc.ID, tc.Function.Name))
	}
	return items, nil
}

// convertTools maps tool definitions to strict function tools.
func convertTools(tools []tool.Definition) ([]responses.ToolUnionParam, error) {
	result := make([]responses.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		params, err := schemaToMap(t.Parameters.Strict())
		if err != nil {
			return nil, fmt.Errorf("tool %s: %w", t.Name, err)
		}
		fn := &responses.FunctionToolParam{
			Name:       t.Name,
			Parameters: params,
			Strict:     openai.Bool(true),
		}
		if t.Description != "" {
			fn.Description = openai.String(t.Description)
		}
		result = append(result, responses.ToolUnionParam{OfFunction: fn})
	}
	return result, nil
}

// convertResponseFormat maps a structured-output request to a strict JSON schema format.
func convertResponseFormat(rf *provider.ResponseFormat) (responses.ResponseTextConfigParam, error) {
	s, err := schemaToMap(rf.Schema.Strict())
	if err != nil {
		return responses.ResponseTextConfigParam{}, fmt.Errorf("response format: %w", err)
	}
	name := rf.Name
	if name == "" {
		name = "response"
	}
	return responses.ResponseTextConfigParam{
		Format: responses.ResponseFormatTextConfigUnionParam{
			OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
				Name:   name,
				Schema: s,
				Strict: openai.Bool(true),
			},
		},
	}, nil
}

// schemaToMap converts a schema to the map form the SDK expects, keeping each
// value as raw JSON so nothing is re-encoded lossily.
func schemaToMap(s *schema.Schema) (map[string]any, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("failed to encode schema: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode schema: %w", err)
	}
	result := make(map[string]any, len(raw))
	for k, v := range raw {
		result[k] = v
	}
	return result, nil
}

// convertResponse maps a Responses API response to a provider response.
// Failed and incomplete responses are returned as errors.
func convertResponse(r *responses.Response) (*provider.Response, error) {
	switch r.Status {
	case responses.ResponseStatusFailed:
		return nil, fmt.Errorf("response failed: %s: %s", r.Error.Code, r.Error.Message)
	case responses.ResponseStatusIncomplete:
		return nil, fmt.Errorf("response incomplete: %s", r.IncompleteDetails.Reason)
	}

	result := &provider.Response{Usage: &message.Usage{
		PromptTokens:       int(r.Usage.InputTokens),
		CompletionTokens:   int(r.Usage.OutputTokens),
		TotalTokens:        int(r.Usage.TotalTokens),
		CachedPromptTokens: int(r.Usage.InputTokensDetails.CachedTokens),
	}}

	var content strings.Builder
	for _, item := range r.Output {
		switch item.Type {
		case "message":
			if err := appendMessageText(&content, item.AsMessage()); err != nil {
				return nil, err
			}
		case "function_call":
			fc := item.AsFunctionCall()
			result.ToolCalls = append(result.ToolCalls, message.ToolCall{
				ID:       fc.CallID,
				Type:     "function",
				Function: message.FunctionCall{Name: fc.Name, Arguments: fc.Arguments},
			})
		case "reasoning":
			result.ProviderItems = append(result.ProviderItems, json.RawMessage(item.RawJSON()))
		}
	}
	result.Content = content.String()
	return result, nil
}

// appendMessageText adds a message's text to b, failing on a refusal.
func appendMessageText(b *strings.Builder, msg responses.ResponseOutputMessage) error {
	for _, part := range msg.Content {
		switch part.Type {
		case "output_text":
			b.WriteString(part.Text)
		case "refusal":
			return fmt.Errorf("model refused: %s", part.Refusal)
		}
	}
	return nil
}
