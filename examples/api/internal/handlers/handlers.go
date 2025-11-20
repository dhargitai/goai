// Package handlers implements HTTP handlers for AI-powered endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dhargitai/goai/agent"
	"github.com/dhargitai/goai/provider"
)

// Handler contains dependencies for HTTP handlers
type Handler struct {
	provider provider.Provider
}

// NewHandler creates a new handler with the given provider
func NewHandler(provider provider.Provider) *Handler {
	return &Handler{
		provider: provider,
	}
}

// HealthCheck returns the health status of the service
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// AnalyzeTextRequest represents the request for text analysis
type AnalyzeTextRequest struct {
	Text string `json:"text"`
}

// AnalysisResult represents the structured analysis result
type AnalysisResult struct {
	Sentiment string   `json:"sentiment" description:"Overall sentiment (positive, negative, neutral)"`
	Topics    []string `json:"topics" description:"Main topics discussed"`
	KeyPoints []string `json:"key_points" description:"Key points from the text"`
	Summary   string   `json:"summary" description:"Brief summary"`
}

// AnalyzeText performs comprehensive text analysis
func (h *Handler) AnalyzeText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text field is required", http.StatusBadRequest)
		return
	}

	// Create agent for analysis
	deps := struct{}{}
	ag, err := agent.NewAgent[struct{}, AnalysisResult](
		h.provider,
		"You are an expert text analyst. Analyze the given text and provide sentiment, topics, key points, and a summary.",
		deps,
	)
	if err != nil {
		http.Error(w, "Failed to create agent", http.StatusInternalServerError)
		return
	}

	// Run analysis with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := ag.Run(ctx, fmt.Sprintf("Analyze this text:\n\n%s", req.Text))
	if err != nil {
		http.Error(w, "Analysis failed", http.StatusInternalServerError)
		return
	}

	// Return results
	response := map[string]interface{}{
		"analysis": result.Data,
		"usage": map[string]int{
			"prompt_tokens":     result.Usage.PromptTokens,
			"completion_tokens": result.Usage.CompletionTokens,
			"total_tokens":      result.Usage.TotalTokens,
		},
		"cost": result.Cost,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ExtractEntitiesRequest represents the request for entity extraction
type ExtractEntitiesRequest struct {
	Text string `json:"text"`
}

// EntitiesResult represents extracted entities
type EntitiesResult struct {
	People        []string `json:"people" description:"Names of people mentioned"`
	Organizations []string `json:"organizations" description:"Names of organizations"`
	Locations     []string `json:"locations" description:"Locations mentioned"`
	Dates         []string `json:"dates" description:"Dates and times mentioned"`
}

// ExtractEntities extracts named entities from text
func (h *Handler) ExtractEntities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExtractEntitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text field is required", http.StatusBadRequest)
		return
	}

	// Create agent for entity extraction
	deps := struct{}{}
	ag, err := agent.NewAgent[struct{}, EntitiesResult](
		h.provider,
		"You are an expert at extracting named entities from text. Extract all people, organizations, locations, and dates.",
		deps,
	)
	if err != nil {
		http.Error(w, "Failed to create agent", http.StatusInternalServerError)
		return
	}

	// Run extraction with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := ag.Run(ctx, fmt.Sprintf("Extract entities from:\n\n%s", req.Text))
	if err != nil {
		http.Error(w, "Extraction failed", http.StatusInternalServerError)
		return
	}

	// Return results
	response := map[string]interface{}{
		"entities": result.Data,
		"usage": map[string]int{
			"total_tokens": result.Usage.TotalTokens,
		},
		"cost": result.Cost,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SummarizeRequest represents the request for summarization
type SummarizeRequest struct {
	Text      string `json:"text"`
	MaxLength int    `json:"max_length,omitempty"` // Optional: target length in words
	Style     string `json:"style,omitempty"`      // Optional: "brief", "detailed", "bullet-points"
}

// SummaryResult represents a text summary
type SummaryResult struct {
	Summary   string   `json:"summary" description:"The summary of the text"`
	MainIdeas []string `json:"main_ideas" description:"List of main ideas"`
	WordCount int      `json:"word_count" description:"Number of words in summary"`
}

// Summarize creates a summary of the given text
func (h *Handler) Summarize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SummarizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "Text field is required", http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.Style == "" {
		req.Style = "brief"
	}

	// Build system prompt based on style
	systemPrompt := "You are an expert at creating concise summaries. "
	switch req.Style {
	case "brief":
		systemPrompt += "Create brief, concise summaries focusing on the most important points."
	case "detailed":
		systemPrompt += "Create detailed summaries covering all major points and supporting details."
	case "bullet-points":
		systemPrompt += "Create summaries in bullet-point format for easy scanning."
	}

	// Create agent for summarization
	deps := struct{}{}
	ag, err := agent.NewAgent[struct{}, SummaryResult](
		h.provider,
		systemPrompt,
		deps,
	)
	if err != nil {
		http.Error(w, "Failed to create agent", http.StatusInternalServerError)
		return
	}

	// Build user prompt
	userPrompt := fmt.Sprintf("Summarize this text in %s style", req.Style)
	if req.MaxLength > 0 {
		userPrompt += fmt.Sprintf(" (target: ~%d words)", req.MaxLength)
	}
	userPrompt += fmt.Sprintf(":\n\n%s", req.Text)

	// Run summarization with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, err := ag.Run(ctx, userPrompt)
	if err != nil {
		http.Error(w, "Summarization failed", http.StatusInternalServerError)
		return
	}

	// Return results
	response := map[string]interface{}{
		"summary": result.Data,
		"usage": map[string]int{
			"total_tokens": result.Usage.TotalTokens,
		},
		"cost": result.Cost,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
