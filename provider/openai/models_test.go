package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dhargitai/goai/message"
)

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gpt-4o", false},
		{"gpt-4o-mini", false},
		{"gpt-4o-2024-08-06", false},
		{"gpt-4.1-nano", false},
		{"gpt-5-chat-latest", false},
		{"gpt-5.3-chat-latest", false},
		{"o1", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-5", true},
		{"gpt-5-mini", true},
		{"gpt-5.5", true},
		{"gpt-6.1-sol", true},
		{"gpt-12", true},
		{"gpt-x", false},
		{"text-embedding-3-small", false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.want, isReasoningModel(tt.model))
		})
	}
}

func TestLookupPrice(t *testing.T) {
	tests := []struct {
		model     string
		wantInput float64
		wantFound bool
	}{
		{model: "gpt-4o", wantInput: 2.50, wantFound: true},
		{model: "gpt-4o-2024-08-06", wantInput: 2.50, wantFound: true},
		{model: "gpt-4o-mini", wantInput: 0.15, wantFound: true},
		{model: "gpt-4o-mini-2024-07-18", wantInput: 0.15, wantFound: true},
		{model: "gpt-5-mini", wantInput: 0.25, wantFound: true},
		{model: "gpt-5.4-mini", wantInput: 0.75, wantFound: true},
		{model: "gpt-6.1-sol", wantInput: 2.00, wantFound: true},
		{model: "gpt-4o.5", wantFound: false},
		{model: "unknown-model", wantFound: false},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			p, found := lookupPrice(tt.model)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantInput, p.input)
		})
	}
}

func TestEstimateCost(t *testing.T) {
	tests := []struct {
		name  string
		model string
		usage *message.Usage
		want  float64
	}{
		{name: "nil usage", model: "gpt-4o", usage: nil, want: 0},
		{name: "unknown model", model: "mystery", usage: &message.Usage{PromptTokens: 1000}, want: 0},
		{
			name:  "cached input is billed at the cached rate",
			model: "gpt-4o",
			usage: &message.Usage{PromptTokens: 1_000_000, CachedPromptTokens: 400_000, CompletionTokens: 100_000},
			want:  0.6*2.50 + 0.4*1.25 + 0.1*10.00,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.InDelta(t, tt.want, estimateCost(tt.model, tt.usage), 1e-9)
		})
	}
}
