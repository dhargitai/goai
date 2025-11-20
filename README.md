# GoAI - Type-Safe AI Agent Framework for Go

A type-safe AI agent framework for Go (Golang) that mirrors Pydantic AI's core functionality.

## Features

- 🔒 **Type-Safe**: Leverage Go generics for compile-time type safety
- 🚀 **OpenAI Integration**: GPT-4/GPT-4o support with function calling
- 📝 **Structured Outputs**: Automatic JSON schema generation from Go types
- 🔧 **Tool/Function Calling**: Automatic tool registration and execution
- 💉 **Dependency Injection**: Clean dependency management with generics
- 🌊 **Streaming Support**: Stream responses via Go channels
- ⚡ **Performance**: 10-50x faster than Python implementations
- 📦 **Single Binary**: Deploy as a static binary with no dependencies

## Installation

```bash
go get github.com/dhargitai/goai
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dhargitai/goai/agent"
    "github.com/dhargitai/goai/provider/openai"
)

type WeatherInfo struct {
    Temperature float64 `json:"temperature" description:"Temperature in Fahrenheit"`
    Condition   string  `json:"condition" description:"Weather condition"`
}

func main() {
    ctx := context.Background()

    // Create OpenAI provider (set OPENAI_API_KEY environment variable)
    provider, err := openai.New()
    if err != nil {
        log.Fatal(err)
    }

    // Or override defaults with options:
    // provider, err := openai.New(
    //     openai.WithModel("gpt-4-turbo"),
    //     openai.WithAPIKey("your-api-key"),
    // )

    // Create agent with typed output
    deps := struct{}{} // No dependencies needed for this example
    ag, agentErr := agent.NewAgent[struct{}, WeatherInfo](
        provider,
        "You provide weather information in the requested format",
        deps,
    )
    if agentErr != nil {
        log.Fatal(agentErr)
    }

    // Run agent - automatically returns structured output
    result, err := ag.Run(ctx, "What's the weather like in San Francisco?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Temperature: %.1f°F\n", result.Data.Temperature)
    fmt.Printf("Condition: %s\n", result.Data.Condition)
    fmt.Printf("Cost: $%.6f\n", result.Cost)
}
```

## Agent with Dependencies and Tools

```go
package main

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/dhargitai/goai/agent"
    "github.com/dhargitai/goai/provider/openai"
)

type Deps struct {
    DB *sql.DB
}

type UserInfo struct {
    Name  string `json:"name" description:"User's full name"`
    Email string `json:"email" description:"User's email address"`
}

type UserArgs struct {
    UserID int `json:"user_id" description:"The user ID to fetch"`
}

func main() {
    ctx := context.Background()
    db, _ := sql.Open("sqlite3", "users.db")
    defer db.Close()

    provider := openai.New(apiKey, "gpt-4o")
    deps := Deps{DB: db}

    ag, _ := agent.NewAgent[Deps, UserInfo](
        provider,
        "You are a helpful assistant with access to user database",
        deps,
    )

    // Register tool - agent will call this automatically when needed
    ag.Tool("get_user", "Fetch user from database",
        func(ctx agent.RunContext[Deps], args UserArgs) (*UserInfo, error) {
            // Access dependencies via ctx.Deps
            var user UserInfo
            err := ctx.Deps.DB.QueryRow(
                "SELECT name, email FROM users WHERE id = ?",
                args.UserID,
            ).Scan(&user.Name, &user.Email)
            return &user, err
        },
    )

    // Run agent - it will automatically use the tool if needed
    result, err := ag.Run(ctx, "What is the email for user 123?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("User: %s (%s)\n", result.Data.Name, result.Data.Email)
}
```

### Streaming Responses

Stream incremental responses for real-time output:

```go
// Run agent with streaming
streamResult, err := ag.RunStream(ctx, "Write a story about robots")
if err != nil {
    log.Fatal(err)
}
defer streamResult.Close()

// Consume chunks in real-time
go func() {
    for chunk := range streamResult.Chunks() {
        if chunk.Content != "" {
            fmt.Print(chunk.Content) // Print each chunk as it arrives
        }
        if chunk.IsToolExecution {
            fmt.Printf("[Calling tool: %s]\n", chunk.ToolName)
        }
    }
}()

// Get final result
result, err := streamResult.Result()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("\n\nTokens used: %d\n", result.Usage.TotalTokens)
fmt.Printf("Cost: $%.6f\n", result.Cost)
```

## Examples

See the [examples](./examples) directory for complete working examples:

- **[Simple CLI](./examples/simple-cli)** - Command-line assistant demonstrating tool calling and structured outputs (currently available)
- **API Service** - HTTP API service with AI-powered endpoints (coming soon)
- **Data Pipeline** - Batch data processing with structured outputs (coming soon)

## Documentation

- [API Reference](https://pkg.go.dev/github.com/dhargitai/goai)
- [Architecture Overview](./docs/architecture.md)
- [Migration from Pydantic AI](./docs/migration.md)
- [Contributing Guide](./CONTRIBUTING.md)

## Development

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Run all checks
make check

# Build examples
make build
```

## Requirements

- Go 1.22 or higher (for generics support)
- OpenAI API key

## License

MIT License - see [LICENSE](./LICENSE) file for details

## Acknowledgments

Inspired by [Pydantic AI](https://ai.pydantic.dev/) - bringing similar patterns to the Go ecosystem with idiomatic Go design.
