// Package main demonstrates a simple CLI using GoAI with tools.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dhargitai/goai/agent"
	"github.com/dhargitai/goai/provider/openai"
)

// Dependencies for our agent
type AgentDeps struct {
	// In a real app, this would be actual database or API clients
	DatabaseURL string
	StartTime   time.Time
}

// Result structure - what we want the agent to return
type AssistantResult struct {
	Answer   string   `json:"answer" description:"The answer to the user's question"`
	Sources  []string `json:"sources,omitempty" description:"Sources used to answer the question"`
	Metadata string   `json:"metadata,omitempty" description:"Additional metadata about the response"`
}

func main() {
	// Create OpenAI provider (reads OPENAI_API_KEY and OPENAI_MODEL from environment)
	provider, err := openai.New()
	if err != nil {
		log.Fatal(err)
	}

	// Create dependencies
	deps := AgentDeps{
		DatabaseURL: "postgres://localhost:5432/mydb",
		StartTime:   time.Now(),
	}

	// Create system prompt
	systemPrompt := `You are a helpful assistant with access to tools.
You can search a database and get system information.
When answering questions, use the available tools to gather accurate information.
Always provide your answer in the structured format requested.`

	// Create agent
	ag, err := agent.NewAgent[AgentDeps, AssistantResult](
		provider,
		systemPrompt,
		deps,
	)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	// Register tools
	type SearchArgs struct {
		Query string `json:"query" description:"The search query"`
	}

	err = ag.Tool("search_database", "Search the database for information", func(ctx agent.RunContext[AgentDeps], args SearchArgs) (string, error) {
		// In a real app, this would query a database
		// For demo purposes, return mock data
		results := []string{
			fmt.Sprintf("Found 5 results for '%s'", args.Query),
			"Result 1: GoAI is a type-safe AI agent framework",
			"Result 2: It supports structured outputs and tool calling",
			"Result 3: Built with Go generics for compile-time safety",
		}

		return fmt.Sprintf("Database results:\n%s\n%s\n%s\n%s",
			results[0], results[1], results[2], results[3]), nil
	})
	if err != nil {
		log.Fatalf("Failed to register search tool: %v", err)
	}

	err = ag.Tool("get_system_info", "Get system information and uptime", func(ctx agent.RunContext[AgentDeps]) (string, error) {
		uptime := time.Since(ctx.Deps.StartTime)
		return fmt.Sprintf("System uptime: %v\nDatabase: %s",
			uptime.Round(time.Second), ctx.Deps.DatabaseURL), nil
	})
	if err != nil {
		log.Fatalf("Failed to register system info tool: %v", err)
	}

	ctx := context.Background()

	// Example 1: Regular (non-streaming) execution
	fmt.Println("=== Example 1: Regular Execution ===")
	runRegularExample(ctx, ag)

	// Example 2: Streaming execution. Plain-text (string) results stream as
	// they are generated; structured results arrive whole at the end.
	fmt.Println("\n\n=== Example 2: Streaming Execution ===")
	storyteller, err := agent.NewAgent[AgentDeps, string](provider, "You are a concise storyteller.", deps)
	if err != nil {
		log.Fatalf("Failed to create storyteller agent: %v", err)
	}
	runStreamingExample(ctx, storyteller)
}

// runRegularExample demonstrates regular (non-streaming) agent execution
func runRegularExample(ctx context.Context, ag *agent.Agent[AgentDeps, AssistantResult]) {
	userQuery := "What is GoAI and how long has the system been running?"

	fmt.Printf("User: %s\n\n", userQuery)
	fmt.Println("Assistant is thinking...")

	result, err := ag.Run(ctx, userQuery)
	if err != nil {
		log.Fatalf("Agent run failed: %v", err)
	}

	// Print results
	fmt.Printf("\n--- Results ---\n")
	fmt.Printf("Answer: %s\n", result.Data.Answer)
	if len(result.Data.Sources) > 0 {
		fmt.Printf("Sources: %v\n", result.Data.Sources)
	}
	if result.Data.Metadata != "" {
		fmt.Printf("Metadata: %s\n", result.Data.Metadata)
	}

	fmt.Printf("\n--- Usage Stats ---\n")
	fmt.Printf("Tokens: %d (prompt: %d, completion: %d)\n",
		result.Usage.TotalTokens,
		result.Usage.PromptTokens,
		result.Usage.CompletionTokens)
	fmt.Printf("Estimated Cost: $%.6f\n", result.Cost)
	fmt.Printf("Conversation turns: %d\n", len(result.Messages))
}

// runStreamingExample demonstrates streaming agent execution with real-time output
func runStreamingExample(ctx context.Context, ag *agent.Agent[AgentDeps, string]) {
	userQuery := "Write a short story about a Go programmer discovering AI agents"

	fmt.Printf("User: %s\n\n", userQuery)
	fmt.Println("Assistant (streaming): ")

	var result *agent.RunResult[string]
	for ev, err := range ag.RunStream(ctx, userQuery) {
		if err != nil {
			log.Fatalf("Stream failed: %v", err)
		}
		switch {
		case ev.ToolName != "":
			fmt.Printf("\n[Calling tool: %s]\n", ev.ToolName)
		case ev.Content != "":
			fmt.Print(ev.Content)
		case ev.Result != nil:
			result = ev.Result
		}
	}

	fmt.Printf("\n\n--- Usage Stats ---\n")
	fmt.Printf("Tokens: %d (prompt: %d, completion: %d)\n",
		result.Usage.TotalTokens,
		result.Usage.PromptTokens,
		result.Usage.CompletionTokens)
	fmt.Printf("Estimated Cost: $%.6f\n", result.Cost)
}
