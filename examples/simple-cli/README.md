# Simple CLI Example

This example demonstrates the core features of GoAI:

- Creating an agent with typed dependencies and structured outputs
- Registering tools that the LLM can call
- Automatic tool execution loop
- **Streaming responses with real-time output** ⭐
- Structured output parsing
- Usage tracking and cost estimation

## Running the Example

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY="your-api-key-here"
```

2. Run the example:
```bash
go run examples/simple-cli/main.go
```

## What it Demonstrates

The example includes **two demonstrations**:

### Example 1: Regular (Non-Streaming) Execution

Shows synchronous agent execution with tool calling.

**Typed Dependencies:**
The agent has access to `AgentDeps` which includes database configuration and system start time. Tools can access these dependencies via `RunContext`.

**Tool Registration:**
Two tools are registered:
- `search_database`: Searches a (mock) database with a query parameter
- `get_system_info`: Returns system uptime and configuration (no parameters)

**Structured Output:**
The agent returns an `AssistantResult` struct with:
- `Answer`: The main response
- `Sources`: Optional list of sources
- `Metadata`: Optional additional information

**Automatic Tool Execution:**
The agent automatically:
1. Sends the user query to the LLM
2. Detects when the LLM wants to call tools
3. Executes the requested tools
4. Sends results back to the LLM
5. Gets the final structured response

All of this happens in a single `Run()` call!

### Example 2: Streaming Execution ⭐

Shows real-time streaming with a range-over-func iterator. It uses a second
agent with a plain `string` result, because only text results stream token by
token; structured results arrive whole in the final event.

**Streaming Features:**
- Real-time content delivery as tokens arrive
- Tool execution notifications during streaming
- Stop any time with `break` (the request is cancelled, nothing leaks)
- Final result with complete metadata

**How it Works:**
```go
for ev, err := range ag.RunStream(ctx, userQuery) {
    if err != nil {
        log.Fatal(err)
    }
    switch {
    case ev.ToolName != "":
        fmt.Printf("[Calling tool: %s]\n", ev.ToolName)
    case ev.Content != "":
        fmt.Print(ev.Content) // Print as tokens arrive
    case ev.Result != nil:
        fmt.Println(ev.Result.Usage.TotalTokens) // Final result with usage stats
    }
}
```

## Expected Output

```
User: What is GoAI and how long has the system been running?

Assistant is thinking...

=== Results ===
Answer: GoAI is a type-safe AI agent framework built with Go generics that supports structured outputs and tool calling. The system has been running for approximately X seconds.
Sources: [database, system_info]
Metadata: Used 2 tools to gather information

=== Usage Stats ===
Tokens: 450 (prompt: 250, completion: 200)
Estimated Cost: $0.001125
Conversation turns: 6
```

The conversation history would include:
1. System message (instructions)
2. User message (query)
3. Assistant message (tool calls)
4. Tool message (search results)
5. Tool message (system info)
6. Assistant message (final answer)

## Customization

You can modify this example to:
- Add real database queries
- Connect to external APIs
- Add more tools
- Change the output structure
- Adjust the system prompt for different behaviors
