# GoAI - Type-Safe AI Agent Framework for Go

A type-safe AI agent framework for Go (Golang) that mirrors Pydantic AI's core functionality.

## Features

- 🔒 **Type-Safe**: Leverage Go generics for compile-time type safety
- 🚀 **OpenAI Integration**: Built on the official [openai-go](https://github.com/openai/openai-go) SDK and the Responses API
- 📝 **Structured Outputs**: Strict JSON schemas generated from Go types, including nested structs
- 🔧 **Tool/Function Calling**: Automatic tool registration and execution
- 💉 **Dependency Injection**: Clean dependency management with generics
- 🌊 **Streaming Support**: Stream responses with a plain `for ... range` loop
- 🔐 **Stateless**: Nothing is stored on OpenAI's side (`store=false`), so Zero Data Retention accounts work
- 📦 **Single Binary**: Deploy as a static binary with no dependencies

## Supported Models

Any OpenAI model from **gpt-4o onwards**: the gpt-4o and gpt-4.1 families, the
o-series, and GPT-5 and later. The default is `gpt-6.1-sol`
(`openai.DefaultModel`). GoAI adapts each request to the model: temperature is
only sent to non-reasoning models, and reasoning effort and encrypted reasoning
only to reasoning models.

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

type Location struct {
    City    string `json:"city" description:"City name"`
    Country string `json:"country" description:"Country name"`
}

type WeatherInfo struct {
    Location    Location `json:"location"`
    Temperature float64  `json:"temperature" description:"Temperature in Fahrenheit"`
    Condition   string   `json:"condition" enum:"sunny,cloudy,rainy,snowy"`
}

func main() {
    ctx := context.Background()

    // Reads OPENAI_API_KEY (and optionally OPENAI_MODEL) from the environment
    provider, err := openai.New()
    if err != nil {
        log.Fatal(err)
    }

    // Or override defaults with options:
    // provider, err := openai.New(
    //     openai.WithModel("gpt-4o"),
    //     openai.WithAPIKey("your-api-key"),
    // )

    ag, err := agent.NewAgent[struct{}, WeatherInfo](
        provider,
        "You provide weather information in the requested format",
        struct{}{}, // No dependencies needed for this example
    )
    if err != nil {
        log.Fatal(err)
    }

    // Run agent - automatically returns structured output
    result, err := ag.Run(ctx, "What's the weather like in San Francisco?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("%s, %s\n", result.Data.Location.City, result.Data.Location.Country)
    fmt.Printf("Temperature: %.1f°F (%s)\n", result.Data.Temperature, result.Data.Condition)
    fmt.Printf("Cost: $%.6f\n", result.Cost)
}
```

### Schema Tags

Result and tool-argument types are described to the model with struct tags:

| Tag | Effect |
|---|---|
| `json:"name"` | Property name; `json:"-"` skips the field |
| `description:"..."` | Description shown to the model |
| `enum:"a,b,c"` | Restricts the value to the listed options |

Non-pointer fields are required. Pointer fields and fields tagged `omitempty`
are optional: in strict mode the model sends `null` for them.

## Agent with Dependencies and Tools

```go
package main

import (
    "context"
    "database/sql"
    "fmt"
    "log"

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

    db, err := sql.Open("sqlite3", "users.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    provider, err := openai.New(openai.WithModel("gpt-4o"))
    if err != nil {
        log.Fatal(err)
    }

    ag, err := agent.NewAgent[Deps, UserInfo](
        provider,
        "You are a helpful assistant with access to user database",
        Deps{DB: db},
    )
    if err != nil {
        log.Fatal(err)
    }

    // Register tool - agent will call this automatically when needed.
    // The parameter schema sent to the model is generated from UserArgs.
    err = ag.Tool("get_user", "Fetch user from database",
        func(ctx agent.RunContext[Deps], args UserArgs) (*UserInfo, error) {
            // Access dependencies via ctx.Deps
            var user UserInfo
            err := ctx.Deps.DB.QueryRowContext(ctx.Ctx,
                "SELECT name, email FROM users WHERE id = ?",
                args.UserID,
            ).Scan(&user.Name, &user.Email)
            return &user, err
        },
    )
    if err != nil {
        log.Fatal(err)
    }

    result, err := ag.Run(ctx, "What is the email for user 123?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("User: %s (%s)\n", result.Data.Name, result.Data.Email)
}
```

## Streaming Responses

`RunStream` returns an iterator. Text arrives as it is generated, tool calls are
announced before they run, and the last event carries the final result.
Breaking out of the loop cancels the request; there is nothing to close.

```go
// A string result streams token by token. Structured results arrive whole
// in the final event.
storyteller, err := agent.NewAgent[struct{}, string](provider, "You tell short stories", struct{}{})
if err != nil {
    log.Fatal(err)
}

for ev, err := range storyteller.RunStream(ctx, "Write a story about robots") {
    if err != nil {
        log.Fatal(err)
    }
    switch {
    case ev.Content != "":
        fmt.Print(ev.Content) // Print each chunk as it arrives
    case ev.ToolName != "":
        fmt.Printf("[Calling tool: %s]\n", ev.ToolName)
    case ev.Result != nil:
        fmt.Printf("\n\nTokens used: %d\n", ev.Result.Usage.TotalTokens)
        fmt.Printf("Cost: $%.6f\n", ev.Result.Cost)
    }
}
```

## Provider Options

| Option | Purpose |
|---|---|
| `WithModel(model)` | Model to use (overrides `OPENAI_MODEL`) |
| `WithAPIKey(key)` | API key (overrides `OPENAI_API_KEY`) |
| `WithReasoningEffort(effort)` | Reasoning effort for reasoning models: `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max` |
| `WithBaseURL(url)` | Custom endpoint, e.g. a proxy |
| `WithHTTPClient(client)` | Custom `*http.Client` |

`result.Cost` is estimated from OpenAI's published standard-tier prices for the
model. It is 0 for models whose price GoAI doesn't know.

## Examples

See the [examples](./examples) directory for complete working examples:

- **[Simple CLI](./examples/simple-cli)** - Command-line assistant demonstrating tool calling, structured outputs and streaming
- **[API Service](./examples/api)** - HTTP API service with AI-powered endpoints
- **[Data Pipeline](./examples/pipeline)** - Batch data processing with structured outputs

## Documentation

- [API Reference](https://pkg.go.dev/github.com/dhargitai/goai)
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

# Scan for known vulnerabilities
make vuln

# Run live tests against the OpenAI API (needs OPENAI_API_KEY; costs a little)
make integration

# Build examples
make build
```

## Requirements

- Go 1.26 or higher
- OpenAI API key

## License

MIT License - see [LICENSE](./LICENSE) file for details

## Acknowledgments

Inspired by [Pydantic AI](https://ai.pydantic.dev/) - bringing similar patterns to the Go ecosystem with idiomatic Go design.
