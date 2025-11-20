# Data Pipeline Example

This example demonstrates batch data processing with AI agents using Go's concurrency patterns.

## Features

- 📊 **Batch Processing** - Process large datasets efficiently
- ⚡ **Concurrent Workers** - Worker pool pattern with configurable parallelism
- 📁 **CSV Input/Output** - Read from and write to CSV files
- 🎯 **Text Classification** - Categorize text with confidence scores
- 📈 **Progress Tracking** - Real-time logging of processing status
- 💰 **Cost Tracking** - Per-record and total cost estimation

## What It Does

The pipeline:
1. Reads text records from a CSV file
2. Classifies each record using an AI agent
3. Extracts categories, subcategories, keywords, and confidence
4. Writes results to an output CSV with timing and cost data
5. Prints summary statistics

### Example Classification

**Input:**
```
"New AI framework released for Go developers. Features include type-safe agents..."
```

**Output:**
```json
{
  "category": "tech",
  "subcategory": "software_development",
  "confidence": "high",
  "keywords": ["AI", "Go", "framework", "type-safe", "agents"]
}
```

## Running the Example

### Prerequisites

1. Set your OpenAI API key:
```bash
export OPENAI_API_KEY="your-api-key-here"
```

2. Ensure sample data exists:
```bash
ls examples/pipeline/data/input.csv
```

### Basic Usage

```bash
cd examples/pipeline
go run cmd/pipeline/main.go
```

This will:
- Read from `data/input.csv`
- Process with 5 concurrent workers
- Write results to `data/output.csv`

### Custom Input/Output

```bash
go run cmd/pipeline/main.go path/to/input.csv path/to/output.csv
```

### Adjust Concurrency

Edit the `workers` variable in `main.go`:

```go
workers := 10 // Increase for more parallelism
```

**Note:** Higher concurrency = faster processing but higher API rate limit usage.

## Input Format

CSV file with header and two columns:

```csv
id,text
1,"First text to classify..."
2,"Second text to classify..."
```

## Output Format

CSV with classification results:

```csv
id,text,category,subcategory,confidence,keywords,process_time,cost
1,"First text...","tech","software","high","[""AI"",""Go""]",0.523,0.000145
2,"Second text...","business","finance","high","[""market"",""stocks""]",0.487,0.000132
```

## Architecture

### Worker Pool Pattern

```
                    ┌─────────┐
   Input CSV ──────>│  Jobs   │
                    │ Channel │
                    └────┬────┘
                         │
          ┌──────────────┼──────────────┐
          │              │              │
      ┌───▼───┐      ┌───▼───┐      ┌───▼───┐
      │Worker │      │Worker │      │Worker │
      │   1   │      │   2   │  ... │   N   │
      └───┬───┘      └───┬───┘      └───┬───┘
          │              │              │
          └──────────────┼──────────────┘
                         │
                    ┌────▼────┐
                    │ Results │
                    │ Channel │
                    └─────────┘
                         │
                    Output CSV
```

### Key Patterns

**Worker Pool:**
- Configurable number of concurrent workers
- Each worker processes records independently
- Shared job queue for load distribution
- Results collected via results channel

**Error Handling:**
- Failed records written to output with error info
- Processing continues on errors
- Timeout protection (30s per record)

**Resource Management:**
- Single agent instance per worker (reused across records)
- Proper channel closing for graceful shutdown
- Context-based cancellation for each record

## Performance Considerations

### Optimizing Throughput

**Adjust Workers:**
```go
workers := 10  // More workers = higher throughput
```

**Trade-offs:**
- More workers = faster processing
- But: API rate limits may throttle requests
- But: Higher memory usage
- Optimal: 5-10 workers for most use cases

### Cost Management

**Track Costs:**
The pipeline tracks per-record and total costs. Example summary:

```
=== Processing Summary ===
Total records: 100
Total time: 52.3s
Average time per record: 0.52s
Total cost: $0.014500
Average cost per record: $0.000145

=== Category Distribution ===
tech: 35 (35.0%)
business: 25 (25.0%)
health: 20 (20.0%)
science: 15 (15.0%)
other: 5 (5.0%)
```

## Customization

### Change Classification Schema

Edit the `ClassificationResult` struct:

```go
type ClassificationResult struct {
    Sentiment  string   `json:"sentiment" description:"positive, negative, neutral"`
    Language   string   `json:"language" description:"detected language"`
    Entities   []string `json:"entities" description:"named entities found"`
}
```

Update the system prompt accordingly.

### Add Custom Tools

```go
// In processWorker, before processing loop:
ag.Tool("lookup_database", "Look up additional context",
    func(ctx agent.RunContext[struct{}], query string) (string, error) {
        // Custom logic here
        return "database results", nil
    },
)
```

### Process Different Formats

**JSON Input:**
```go
func readInputJSON(filename string) ([]InputRecord, error) {
    // Read and parse JSON
}
```

**Database Input:**
```go
func readFromDatabase(db *sql.DB) ([]InputRecord, error) {
    // Query database
}
```

## Production Deployment

### Monitoring

Add:
- Progress bars (e.g., using `github.com/schollz/progressbar`)
- Metrics collection (Prometheus)
- Error rate tracking
- API quota monitoring

### Scaling

**Horizontal Scaling:**
- Split large datasets into chunks
- Run multiple pipeline instances
- Aggregate results

**Vertical Scaling:**
- Increase worker count
- Use faster machine types
- Optimize batch sizes

### Error Recovery

Add checkpointing:
```go
// Write progress periodically
// Resume from last checkpoint on failure
// Retry failed records with exponential backoff
```

## Example Output

```
2025-11-20 14:30:00 Starting pipeline with 5 workers
2025-11-20 14:30:00 Input: data/input.csv, Output: data/output.csv
2025-11-20 14:30:00 Loaded 10 records
2025-11-20 14:30:01 Worker 0: processed record 1 in 0.52s (category: tech)
2025-11-20 14:30:01 Worker 1: processed record 2 in 0.48s (category: business)
2025-11-20 14:30:02 Worker 2: processed record 3 in 0.55s (category: health)
...
2025-11-20 14:30:06
=== Processing Summary ===
Total records: 10
Total time: 5.2s
Average time per record: 0.52s
Total cost: $0.001450
Average cost per record: $0.000145

=== Category Distribution ===
tech: 3 (30.0%)
business: 2 (20.0%)
health: 2 (20.0%)
science: 2 (20.0%)
other: 1 (10.0%)
```

## License

Same as parent project.
