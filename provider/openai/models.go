package openai

import (
	"strconv"
	"strings"

	"github.com/dhargitai/goai/message"
)

// isReasoningModel reports whether model is a reasoning model (o-series,
// GPT-5 and later). Reasoning models reject sampling parameters such as
// temperature, accept a reasoning effort, and return reasoning items that must
// be replayed between turns. The "-chat" variants are non-reasoning models.
func isReasoningModel(model string) bool {
	if strings.Contains(model, "-chat") {
		return false
	}
	for _, prefix := range []string{"o1", "o3", "o4"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}

	rest, ok := strings.CutPrefix(model, "gpt-")
	if !ok {
		return false
	}
	major, _, _ := strings.Cut(rest, "-")
	major, _, _ = strings.Cut(major, ".")
	n, err := strconv.Atoi(major)
	return err == nil && n >= 5
}

// price is a model's standard-tier price in USD per 1M tokens.
type price struct {
	input, cachedInput, output float64
}

// prices holds OpenAI standard-tier text prices (short-context rates),
// taken from https://developers.openai.com/api/docs/pricing in October 2026.
// Dated snapshots resolve to their base model by longest prefix match.
var prices = map[string]price{
	"gpt-6-astra":   {10.00, 1.00, 50.00},
	"gpt-6.1-sol":   {2.00, 0.10, 10.00},
	"gpt-6.1-terra": {1.00, 0.05, 5.00},
	"gpt-6-sol":     {2.00, 0.20, 10.00},
	"gpt-6-luna":    {0.10, 0.01, 0.50},
	"gpt-5.6-sol":   {4.00, 0.40, 20.00},
	"gpt-5.6-terra": {2.00, 0.20, 12.00},
	"gpt-5.6-luna":  {0.20, 0.02, 1.20},
	"gpt-5.5":       {5.00, 0.50, 30.00},
	"gpt-5.4":       {2.50, 0.25, 15.00},
	"gpt-5.4-pro":   {30.00, 30.00, 180.00},
	"gpt-5.4-mini":  {0.75, 0.075, 4.50},
	"gpt-5.4-nano":  {0.20, 0.02, 1.25},
	"gpt-5.2":       {1.75, 0.175, 14.00},
	"gpt-5.1":       {1.25, 0.125, 10.00},
	"gpt-5":         {1.25, 0.125, 10.00},
	"gpt-5-mini":    {0.25, 0.025, 2.00},
	"gpt-5-nano":    {0.05, 0.005, 0.40},
	"gpt-4.1":       {2.00, 0.50, 8.00},
	"gpt-4.1-mini":  {0.40, 0.10, 1.60},
	"gpt-4.1-nano":  {0.10, 0.025, 0.40},
	"gpt-4o":        {2.50, 1.25, 10.00},
	"gpt-4o-mini":   {0.15, 0.075, 0.60},
	"o4-mini":       {1.10, 0.275, 4.40},
	"o3":            {2.00, 0.50, 8.00},
	"o1":            {15.00, 7.50, 60.00},
}

// lookupPrice finds the price of model, matching dated snapshots such as
// "gpt-4o-2024-08-06" to their base model by the longest known prefix.
func lookupPrice(model string) (price, bool) {
	best := ""
	for name := range prices {
		if len(name) > len(best) && (model == name || strings.HasPrefix(model, name+"-")) {
			best = name
		}
	}
	p, ok := prices[best]
	return p, ok
}

// estimateCost returns the USD cost of usage on model, or 0 if the price is unknown.
func estimateCost(model string, usage *message.Usage) float64 {
	p, ok := lookupPrice(model)
	if !ok || usage == nil {
		return 0
	}
	cached := float64(usage.CachedPromptTokens)
	uncached := float64(usage.PromptTokens) - cached
	output := float64(usage.CompletionTokens)
	return (uncached*p.input + cached*p.cachedInput + output*p.output) / 1_000_000
}
