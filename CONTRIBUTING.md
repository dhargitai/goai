# Contributing to GoAI

Thank you for your interest in contributing to GoAI! This document provides guidelines and instructions for contributors.

## Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help maintain a welcoming environment

## Getting Started

### Prerequisites

- Go 1.26 or later
- OpenAI API key (for testing)
- Familiarity with Go best practices

### Development Setup

1. **Fork and clone the repository:**
```bash
git clone https://github.com/dhargitai/goai.git
cd goai
```

2. **Install dependencies:**
```bash
go mod download
```

3. **Run tests:**
```bash
go test ./...
```

4. **Run with race detector:**
```bash
go test ./... -race
```

## Development Workflow

### Before Starting

1. **Check existing issues** - Look for related issues or feature requests
2. **Discuss major changes** - Open an issue for discussion before implementing large features
3. **Follow Go idioms** - Read Effective Go and follow standard patterns

### Making Changes

1. **Create a feature branch:**
```bash
git checkout -b feature/your-feature-name
```

2. **Make your changes** following the coding guidelines below

3. **Write tests** for new functionality (aim for 80%+ coverage)

4. **Run the full test suite:**
```bash
make test
```

5. **Format your code:**
```bash
gofmt -w .
go vet ./...
```

6. **Commit your changes:**
```bash
git add .
git commit -m "Add feature: brief description"
```

7. **Push and create a pull request:**
```bash
git push origin feature/your-feature-name
```

## Coding Guidelines

### Go Best Practices

**MUST follow:**
- ✅ Run `gofmt` on all code
- ✅ Pass `go vet ./...` with no warnings
- ✅ Check all errors (never use `_` for error returns)
- ✅ Use `context.Context` as first parameter in functions
- ✅ Clean up resources with `defer`
- ✅ Handle goroutine cleanup properly
- ✅ Write table-driven tests

**Naming Conventions:**
- Exported types/functions: `CapitalCase`
- Unexported: `camelCase`
- Interfaces: `-er` suffix for single-method (e.g., `Reader`)
- Constructors: `New + TypeName` (e.g., `NewAgent`)

**Error Handling:**
```go
// ✅ GOOD: Check and wrap errors
result, err := operation()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// ❌ BAD: Ignore errors
result, _ := operation()
```

**Context Usage:**
```go
// ✅ GOOD: Context as first parameter
func DoWork(ctx context.Context, input string) error {
    // Respect context cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    return work(ctx, input)
}
```

**Resource Cleanup:**
```go
// ✅ GOOD: Use defer immediately
file, err := os.Open("file.txt")
if err != nil {
    return err
}
defer file.Close()
```

### Project-Specific Guidelines

**Package Structure:**
- `agent/` - Core agent implementation
- `provider/` - LLM provider interfaces and implementations
- `tool/` - Tool registration and execution
- `message/` - Message type definitions
- `schema/` - JSON schema generation
- `examples/` - Working examples

**Generics Usage:**
- Use `any` constraint unless specific constraints needed
- Document type parameters clearly
- Provide examples for generic types

**Testing:**
- Unit tests alongside implementation (`*_test.go`)
- Table-driven tests for multiple scenarios
- Mock provider for agent testing
- Integration tests in `examples/`

## Testing Guidelines

### Writing Tests

**Table-Driven Pattern (MANDATORY):**
```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name    string
        input   InputType
        want    OutputType
        wantErr bool
    }{
        {
            name:    "happy path",
            input:   InputType{Data: "valid"},
            want:    OutputType{Result: "expected"},
            wantErr: false,
        },
        {
            name:    "error case",
            input:   InputType{Data: "invalid"},
            want:    OutputType{},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Feature(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("Feature() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("Feature() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

**Coverage Requirements:**
- Core packages: 80%+ coverage
- New features: Include tests in the same PR
- Bug fixes: Add regression test

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test ./... -cover

# With race detector
go test ./... -race

# Specific package
go test ./agent -v

# Using Makefile
make test
make test-race
make coverage
```

## Documentation

### GoDoc Comments

**Package Documentation:**
```go
// Package agent provides type-safe AI agents with structured outputs.
// It combines LLM providers, tools, and dependency injection.
package agent
```

**Type Documentation:**
```go
// Agent represents an AI agent with typed dependencies and results.
// Deps is the dependency type injected into tool handlers.
// Result is the structured output type expected from the agent.
type Agent[Deps any, Result any] struct {
    // ...
}
```

**Function Documentation:**
```go
// NewAgent creates a new agent with the given provider and configuration.
// The Result type is inferred from the generic parameter and used to generate
// a JSON schema for structured output validation.
//
// Example:
//   agent, err := NewAgent[Deps, Result](provider, systemPrompt, deps)
func NewAgent[Deps any, Result any](...) (*Agent[Deps, Result], error) {
    // ...
}
```

### README Updates

If your change affects usage:
1. Update relevant sections in README.md
2. Add code examples if introducing new features
3. Update the Quick Start if changing core APIs

## Pull Request Process

### Before Submitting

**Checklist:**
- [ ] All tests pass (`go test ./...`)
- [ ] Race detector passes (`go test ./... -race`)
- [ ] Code formatted (`gofmt -w .`)
- [ ] No vet warnings (`go vet ./...`)
- [ ] Documentation updated
- [ ] Tests added for new functionality
- [ ] Examples updated if needed

### PR Description

Include:
1. **What** - Brief description of changes
2. **Why** - Motivation and context
3. **How** - Implementation approach
4. **Testing** - How you tested the changes

**Example:**
```markdown
## Changes
Add streaming support for agent responses

## Motivation
Users need real-time feedback for long-running AI operations

## Implementation
- Added StreamResult[Result] type with channels
- Implemented RunStream() method
- Added tests for streaming behavior

## Testing
- Unit tests with mock provider
- Integration test with simple-cli example
- Race detector passes
```

### Review Process

1. Maintainer will review within 1-2 business days
2. Address review comments
3. Once approved, maintainer will merge

## Project Structure

```
goai/
├── agent/              # Core agent implementation
│   ├── agent.go       # Agent[Deps, Result] type
│   ├── agent_test.go  # Comprehensive tests
│   └── ...
├── provider/          # LLM providers
│   ├── provider.go    # Provider interface
│   ├── openai/        # OpenAI implementation
│   └── mock/          # Mock for testing
├── tool/              # Tool system
│   ├── tool.go        # Tool registration
│   ├── tool_test.go
│   └── ...
├── message/           # Message types
├── schema/            # JSON schema generation
├── examples/          # Working examples
│   ├── simple-cli/
│   ├── api/
│   └── pipeline/
├── CONTRIBUTING.md    # This file
└── README.md          # Main documentation
```

## Common Tasks

### Adding a New Provider

1. Create `provider/newprovider/` directory
2. Implement `provider.Provider` interface
3. Add tests with table-driven pattern
4. Update README with usage example
5. Add example in `examples/`

### Adding a New Feature

1. Discuss in an issue first
2. Write tests before implementation (TDD)
3. Keep changes focused and minimal
4. Update documentation
5. Add example if user-facing

### Fixing a Bug

1. Write a failing test that reproduces the bug
2. Fix the bug
3. Verify test now passes
4. Check for similar issues elsewhere

## Getting Help

- **Questions**: Open a discussion issue
- **Bugs**: Open a bug report issue
- **Features**: Open a feature request issue
- **Chat**: [Community channel link if available]

## Release Process

(For maintainers)

1. Update version in `go.mod`
2. Update CHANGELOG.md
3. Create and push tag:
```bash
git tag v0.2.0
git push origin v0.2.0
```
4. GitHub Actions will create release

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (see LICENSE file).

## Thank You!

Your contributions make GoAI better for everyone. Thank you for taking the time to contribute!
