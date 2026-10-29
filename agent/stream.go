package agent

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/dhargitai/goai/provider"
)

// StreamEvent is a single update from a streaming agent run. Exactly one
// field is set per event.
type StreamEvent[Result any] struct {
	// Content is incremental text from the model
	Content string

	// ToolName is set just before the named tool is executed
	ToolName string

	// Result is the final result, set on the last event of a successful run
	Result *RunResult[Result]
}

// RunStream executes the agent and streams its progress. The last event of a
// successful run carries the Result. Breaking out of the loop stops the run and
// cancels any in-flight provider request; no goroutines are left behind.
//
// Example:
//
//	for ev, err := range ag.RunStream(ctx, "Tell me a story") {
//	    if err != nil {
//	        return err
//	    }
//	    switch {
//	    case ev.Content != "":
//	        fmt.Print(ev.Content)
//	    case ev.ToolName != "":
//	        fmt.Printf("[calling %s]\n", ev.ToolName)
//	    case ev.Result != nil:
//	        fmt.Println(ev.Result.Data)
//	    }
//	}
func (a *Agent[Deps, Result]) RunStream(ctx context.Context, userPrompt string) iter.Seq2[*StreamEvent[Result], error] {
	return func(yield func(*StreamEvent[Result], error) bool) {
		state, err := a.start(userPrompt)
		if err != nil {
			yield(nil, err)
			return
		}
		ctx := context.WithValue(ctx, usageKey{}, state.usage)
		onTool := func(name string) bool {
			return yield(&StreamEvent[Result]{ToolName: name}, nil)
		}

		for iteration := 1; iteration <= maxIterations; iteration++ {
			resp, ok := a.streamStep(ctx, state, iteration, yield)
			if !ok {
				return
			}

			done, err := a.handleResponse(ctx, state, resp, onTool)
			if errors.Is(err, errStopped) {
				return // the consumer stopped iterating
			}
			if err != nil || done {
				result, err := a.finish(state, resp, err)
				if err != nil {
					yield(nil, err)
					return
				}
				yield(&StreamEvent[Result]{Result: result}, nil)
				return
			}
		}

		yield(nil, fmt.Errorf("max iterations (%d) reached without completion", maxIterations))
	}
}

// streamStep performs one streaming provider request, forwarding text chunks.
// It returns the final response, or false if the run must stop because of an
// error (already yielded) or because the consumer stopped iterating.
func (a *Agent[Deps, Result]) streamStep(
	ctx context.Context,
	state *runState,
	iteration int,
	yield func(*StreamEvent[Result], error) bool,
) (*provider.Response, bool) {
	for chunk, err := range a.provider.Stream(ctx, a.buildRequest(state)) {
		if err != nil {
			yield(nil, fmt.Errorf("provider stream failed (iteration %d): %w", iteration, err))
			return nil, false
		}
		if chunk.Response != nil {
			return chunk.Response, true
		}
		// Structured output arrives as JSON fragments; only plain-text
		// results are useful to show incrementally.
		if chunk.Content != "" && a.resultIsString {
			if !yield(&StreamEvent[Result]{Content: chunk.Content}, nil) {
				return nil, false
			}
		}
	}

	yield(nil, fmt.Errorf("provider stream ended without a final response (iteration %d)", iteration))
	return nil, false
}
