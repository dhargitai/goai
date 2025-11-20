# API Service Example

This example demonstrates building an HTTP API service with AI-powered endpoints using GoAI.

## Features

- 🌐 **RESTful HTTP API** with multiple AI-powered endpoints
- 📝 **Text Analysis** - Sentiment, topics, key points extraction
- 🏷️ **Entity Extraction** - People, organizations, locations, dates
- 📋 **Summarization** - Flexible summarization with multiple styles
- ⚡ **Production Ready** - Graceful shutdown, timeouts, logging
- 🐳 **Docker Support** - Containerized deployment

## API Endpoints

### Health Check
```bash
GET /health
```

Returns service health status.

**Response:**
```json
{
  "status": "ok",
  "time": "2025-11-20T13:00:00Z"
}
```

### Text Analysis
```bash
POST /api/analyze
Content-Type: application/json

{
  "text": "Your text here..."
}
```

Performs comprehensive text analysis including sentiment, topics, key points, and summary.

**Response:**
```json
{
  "analysis": {
    "sentiment": "positive",
    "topics": ["AI", "Go", "programming"],
    "key_points": ["Type-safe framework", "Production ready"],
    "summary": "Brief summary of the text..."
  },
  "usage": {
    "prompt_tokens": 150,
    "completion_tokens": 85,
    "total_tokens": 235
  },
  "cost": 0.000588
}
```

### Entity Extraction
```bash
POST /api/extract
Content-Type: application/json

{
  "text": "Apple Inc. announced in San Francisco on January 15th..."
}
```

Extracts named entities (people, organizations, locations, dates).

**Response:**
```json
{
  "entities": {
    "people": ["Steve Jobs"],
    "organizations": ["Apple Inc."],
    "locations": ["San Francisco"],
    "dates": ["January 15th"]
  },
  "usage": {
    "total_tokens": 180
  },
  "cost": 0.000450
}
```

### Text Summarization
```bash
POST /api/summarize
Content-Type: application/json

{
  "text": "Long text to summarize...",
  "max_length": 100,
  "style": "brief"
}
```

Creates a summary of the text.

**Parameters:**
- `text` (required): Text to summarize
- `max_length` (optional): Target length in words
- `style` (optional): "brief", "detailed", or "bullet-points" (default: "brief")

**Response:**
```json
{
  "summary": {
    "summary": "The main summary text...",
    "main_ideas": ["Idea 1", "Idea 2", "Idea 3"],
    "word_count": 95
  },
  "usage": {
    "total_tokens": 200
  },
  "cost": 0.000500
}
```

## Running the Example

### Option 1: Local Development

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY="your-api-key-here"
```

2. Run the server:
```bash
cd examples/api
go run cmd/server/main.go
```

The server will start on `http://localhost:8080`.

### Option 2: With Custom Port

```bash
export OPENAI_API_KEY="your-api-key-here"
export PORT=3000
go run cmd/server/main.go
```

### Option 3: Docker

1. Build the image (from repository root):
```bash
# Must build from repository root due to module dependencies
cd ../../  # Navigate to repository root
docker build -f examples/api/Dockerfile -t goai-api .
```

2. Run the container:
```bash
docker run -p 8080:8080 -e OPENAI_API_KEY="your-key" goai-api
```

**Note:** The Dockerfile must be built from the repository root because the example uses a local `replace` directive to reference the parent `goai` module.

## Testing the API

### Using curl

**Analyze text:**
```bash
curl -X POST http://localhost:8080/api/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "GoAI is an amazing type-safe AI framework for Go. It provides structured outputs, tool calling, and streaming support. The performance is excellent!"
  }'
```

**Extract entities:**
```bash
curl -X POST http://localhost:8080/api/extract \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Apple Inc. CEO Tim Cook announced new products in Cupertino on September 12, 2025."
  }'
```

**Summarize:**
```bash
curl -X POST http://localhost:8080/api/summarize \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Very long article text here...",
    "style": "bullet-points",
    "max_length": 50
  }'
```

## Architecture

### Project Structure
```
examples/api/
├── cmd/
│   └── server/
│       └── main.go          # Server entry point
├── internal/
│   └── handlers/
│       └── handlers.go      # HTTP handlers
├── go.mod
├── Dockerfile
└── README.md
```

### Key Patterns

**Dependency Injection:**
The `Handler` struct holds the provider and is passed to all HTTP handlers.

**Request Timeouts:**
Each endpoint uses a 30-second context timeout to prevent hanging requests.

**Structured Responses:**
All AI responses use typed structs that are automatically validated against JSON schemas.

**Graceful Shutdown:**
The server handles SIGTERM/SIGINT and allows 30 seconds for in-flight requests to complete.

**Logging Middleware:**
All requests are logged with method, path, and duration.

## Production Considerations

### Error Handling
- Input validation on all requests
- Timeout protection (30s per request)
- Graceful error responses with appropriate status codes

### Performance
- Connection pooling (Go standard library handles this)
- Context-based cancellation
- Efficient JSON encoding/decoding

### Monitoring
Add these for production:
- Prometheus metrics endpoint
- Structured logging (e.g., zerolog)
- Request tracing (e.g., OpenTelemetry)
- Rate limiting per client

### Security
- Add authentication middleware
- Validate and sanitize inputs
- Rate limiting
- CORS configuration if needed
- TLS/HTTPS in production

## Extending the Example

### Add More Endpoints

```go
func (h *Handler) YourNewEndpoint(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    // 2. Create agent with appropriate system prompt
    // 3. Run agent with timeout
    // 4. Return structured response
}
```

### Add Database Integration

```go
type Handler struct {
    provider provider.Provider
    db       *sql.DB  // Add database
}

// Use database in tool calls or for caching results
```

### Add Streaming Endpoint

```go
func (h *Handler) StreamingChat(w http.ResponseWriter, r *http.Request) {
    // Use agent.RunStream() and write chunks to http.ResponseWriter
    // with Server-Sent Events (SSE) or WebSocket
}
```

## License

Same as parent project.
