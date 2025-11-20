// Package main demonstrates batch data processing with AI agents.
package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/dhargitai/goai/agent"
	"github.com/dhargitai/goai/provider/openai"
)

// InputRecord represents a row from the input CSV
type InputRecord struct {
	ID   string
	Text string
}

// ClassificationResult represents the AI classification output
type ClassificationResult struct {
	Category    string   `json:"category" description:"Primary category (tech, business, health, science, other)"`
	Subcategory string   `json:"subcategory" description:"More specific subcategory"`
	Confidence  string   `json:"confidence" description:"Confidence level (high, medium, low)"`
	Keywords    []string `json:"keywords" description:"Key terms identified"`
}

// OutputRecord combines input with AI results
type OutputRecord struct {
	ID          string
	Text        string
	Category    string
	Subcategory string
	Confidence  string
	Keywords    string
	ProcessTime float64
	Cost        float64
}

func main() {
	// Configuration
	inputFile := "data/input.csv"
	outputFile := "data/output.csv"
	workers := 5 // Number of concurrent workers

	if len(os.Args) > 1 {
		inputFile = os.Args[1]
	}
	if len(os.Args) > 2 {
		outputFile = os.Args[2]
	}

	log.Printf("Starting pipeline with %d workers", workers)
	log.Printf("Input: %s, Output: %s", inputFile, outputFile)

	// Read input data
	records, err := readInputCSV(inputFile)
	if err != nil {
		log.Fatalf("Failed to read input: %v", err)
	}
	log.Printf("Loaded %d records", len(records))

	// Process records with worker pool
	results := processWithWorkerPool(records, workers)

	// Write results
	if err := writeOutputCSV(outputFile, results); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}

	// Print summary
	printSummary(results)
}

// readInputCSV reads records from CSV file
func readInputCSV(filename string) ([]InputRecord, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	// Skip header row
	if len(rows) < 2 {
		return nil, fmt.Errorf("input file must have header + data rows")
	}

	records := make([]InputRecord, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) < 2 {
			continue
		}
		records = append(records, InputRecord{
			ID:   row[0],
			Text: row[1],
		})
	}

	return records, nil
}

// processWithWorkerPool processes records concurrently
func processWithWorkerPool(records []InputRecord, workers int) []OutputRecord {
	// Create work channel
	jobs := make(chan InputRecord, len(records))
	results := make(chan OutputRecord, len(records))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			processWorker(workerID, jobs, results)
		}(i)
	}

	// Send jobs
	for _, record := range records {
		jobs <- record
	}
	close(jobs)

	// Wait for completion and close results
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	output := make([]OutputRecord, 0, len(records))
	for result := range results {
		output = append(output, result)
	}

	return output
}

// processWorker is a worker that processes records
func processWorker(id int, jobs <-chan InputRecord, results chan<- OutputRecord) {
	// Create provider (shared across worker's lifetime)
	provider, err := openai.New()
	if err != nil {
		log.Printf("Worker %d: failed to create provider: %v", id, err)
		return
	}

	// Create agent once for this worker
	deps := struct{}{}
	ag, agentErr := agent.NewAgent[struct{}, ClassificationResult](
		provider,
		"You are an expert text classifier. Classify the given text into categories and subcategories. Identify key terms.",
		deps,
	)
	if agentErr != nil {
		log.Printf("Worker %d: failed to create agent: %v", id, agentErr)
		return
	}

	// Process jobs
	for record := range jobs {
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		result, err := ag.Run(ctx, fmt.Sprintf("Classify this text:\n\n%s", record.Text))
		cancel()

		processTime := time.Since(start).Seconds()

		if err != nil {
			log.Printf("Worker %d: failed to process record %s: %v", id, record.ID, err)
			results <- OutputRecord{
				ID:          record.ID,
				Text:        record.Text,
				Category:    "error",
				Subcategory: "processing_failed",
				Confidence:  "low",
				Keywords:    err.Error(),
				ProcessTime: processTime,
				Cost:        0,
			}
			continue
		}

		// Convert keywords slice to string
		keywordsJSON, _ := json.Marshal(result.Data.Keywords)

		results <- OutputRecord{
			ID:          record.ID,
			Text:        record.Text,
			Category:    result.Data.Category,
			Subcategory: result.Data.Subcategory,
			Confidence:  result.Data.Confidence,
			Keywords:    string(keywordsJSON),
			ProcessTime: processTime,
			Cost:        result.Cost,
		}

		log.Printf("Worker %d: processed record %s in %.2fs (category: %s)",
			id, record.ID, processTime, result.Data.Category)
	}
}

// writeOutputCSV writes results to CSV file
func writeOutputCSV(filename string, records []OutputRecord) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{
		"id", "text", "category", "subcategory", "confidence", "keywords", "process_time", "cost",
	}); err != nil {
		return err
	}

	// Write records
	for _, record := range records {
		if err := writer.Write([]string{
			record.ID,
			record.Text,
			record.Category,
			record.Subcategory,
			record.Confidence,
			record.Keywords,
			fmt.Sprintf("%.3f", record.ProcessTime),
			fmt.Sprintf("%.6f", record.Cost),
		}); err != nil {
			return err
		}
	}

	return nil
}

// printSummary prints processing statistics
func printSummary(results []OutputRecord) {
	totalTime := 0.0
	totalCost := 0.0
	categories := make(map[string]int)

	for _, r := range results {
		totalTime += r.ProcessTime
		totalCost += r.Cost
		categories[r.Category]++
	}

	log.Println("\n=== Processing Summary ===")
	log.Printf("Total records: %d", len(results))
	log.Printf("Total time: %.2fs", totalTime)
	log.Printf("Average time per record: %.2fs", totalTime/float64(len(results)))
	log.Printf("Total cost: $%.6f", totalCost)
	log.Printf("Average cost per record: $%.6f", totalCost/float64(len(results)))

	log.Println("\n=== Category Distribution ===")
	for category, count := range categories {
		log.Printf("%s: %d (%.1f%%)", category, count, float64(count)/float64(len(results))*100)
	}
}
