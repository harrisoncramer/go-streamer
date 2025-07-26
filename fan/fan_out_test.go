package fan

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFanOut(t *testing.T) {
	tests := []struct {
		name        string
		input       []any
		numWorkers  int
		useCancel   bool
		cancelAfter time.Duration
		timeout     time.Duration
	}{
		{
			name: "transactions to multiple workers",
			input: []any{
				Transaction{ID: "tx1", Amount: 100.50, UserID: "user1", Type: "deposit", Currency: "USD"},
				Transaction{ID: "tx2", Amount: 250.75, UserID: "user2", Type: "withdrawal", Currency: "USD"},
				Transaction{ID: "tx3", Amount: 500.00, UserID: "user3", Type: "transfer", Currency: "EUR"},
				Transaction{ID: "tx4", Amount: 75.25, UserID: "user1", Type: "deposit", Currency: "GBP"},
			},
			numWorkers: 2,
			timeout:    2 * time.Second,
		},
		{
			name: "accounts to three workers",
			input: []any{
				Account{ID: "acc1", Balance: 1500.00, UserID: "user1", Type: "checking"},
				Account{ID: "acc2", Balance: 2500.50, UserID: "user2", Type: "savings"},
				Account{ID: "acc3", Balance: 750.25, UserID: "user3", Type: "investment"},
			},
			numWorkers: 3,
			timeout:    2 * time.Second,
		},
		{
			name:       "empty input",
			input:      []any{},
			numWorkers: 2,
			timeout:    1 * time.Second,
		},
		{
			name: "context cancellation",
			input: []any{
				Transaction{ID: "tx1", Amount: 100.00, UserID: "user1", Type: "deposit", Currency: "USD"},
				Transaction{ID: "tx2", Amount: 200.00, UserID: "user2", Type: "withdrawal", Currency: "USD"},
			},
			numWorkers:  2,
			useCancel:   true,
			cancelAfter: 10 * time.Millisecond,
			timeout:     1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var cancel context.CancelFunc

			if tt.useCancel {
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
			}

			// Create input channel
			input := make(chan any, len(tt.input))
			for _, item := range tt.input {
				input <- item
			}
			close(input)

			// Start cancel goroutine if needed
			if tt.useCancel {
				go func() {
					time.Sleep(tt.cancelAfter)
					cancel()
				}()
			}

			outputs, _ := FanOut(ctx, FanOutParams[any]{
				Quit:        nil,
				Input:       input,
				WorkerCount: tt.numWorkers,
			})

			if tt.numWorkers <= 0 {
				assert.Nil(t, outputs, "Should return nil for invalid worker count")
				return
			}

			require.Equal(t, tt.numWorkers, len(outputs), "Should return correct number of output channels")

			// Collect results from all workers
			allResults := collectWorkerResults(t, outputs, tt.timeout)

			// Assertions
			if tt.useCancel {
				assert.LessOrEqual(t, len(allResults), len(tt.input), "Should not exceed input count when cancelled")
			} else {
				assert.Equal(t, len(tt.input), len(allResults), "Should receive all input items")
			}

			// Verify all output channels are closed
			for i, output := range outputs {
				select {
				case _, ok := <-output:
					assert.False(t, ok, "Output channel %d should be closed", i)
				default:
					t.Errorf("Output channel %d should be closed and readable", i)
				}
			}
		})
	}
}

func collectWorkerResults(t *testing.T, outputs []<-chan any, timeout time.Duration) []any {
	var allResults []any
	timeoutCh := time.After(timeout)

	// Collect results from all workers
	resultChan := make(chan any)
	var wg sync.WaitGroup

	for i, output := range outputs {
		wg.Add(1)
		go func(workerID int, ch <-chan any) {
			defer wg.Done()
			for item := range ch {
				resultChan <- item
			}
		}(i, output)
	}

	// Close result channel when all workers are done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect all results
	for {
		select {
		case item, ok := <-resultChan:
			if !ok {
				return allResults
			}
			allResults = append(allResults, item)
		case <-timeoutCh:
			t.Fatal("Test timed out")
		}
	}
}
