package streamer

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"
)

// BenchmarkStreamer_IOBound measures the performance of the streamer with varying counts of items with varying delays
func BenchmarkStreamer_IOBound(b *testing.B) {
	itemCounts := []int{10, 50, 100}
	delays := []time.Duration{1 * time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond}
	workerCounts := []int{
		1,
		2,
		4,
		8,
		16,
		32,
		64,
		runtime.NumCPU(),
		runtime.NumCPU() * 2,
		runtime.NumCPU() * 4,
		runtime.NumCPU() * 8,
	}

	for _, itemCount := range itemCounts {
		for _, delay := range delays {
			for _, workers := range workerCounts {
				b.Run(fmt.Sprintf("items-%d/delay-%dms/workers-%d", itemCount, delay.Milliseconds(), workers), func(b *testing.B) {
					b.ResetTimer()
					b.ReportAllocs()

					for b.Loop() {
						streamer, err := NewStreamer(NewStreamerParams[int, string]{
							WorkerCount: workers,
							Work: func(ctx context.Context, input int) (string, error) {
								// Simulate I/O-bound work (network call, DB query, etc.)
								select {
								case <-time.After(delay):
									return fmt.Sprintf("result-%d", input), nil
								case <-ctx.Done():
									return "", ctx.Err()
								}
							},
						})
						if err != nil {
							b.Fatal(err)
						}

						ctx := b.Context()
						inputChan := make(chan int, itemCount)

						// Generate test data
						go func() {
							defer close(inputChan)
							for j := range itemCount {
								inputChan <- j
							}
						}()

						results, errs, err := streamer.Stream(ctx, inputChan)
						if err != nil {
							b.Fatal(err)
						}

						// Consume results
						go func() {
							for range results {
								// consume
							}
						}()

						go func() {
							for err := range errs {
								if err != nil {
									b.Errorf("Unexpected error: %v", err)
								}
							}
						}()

						streamer.Flush()
					}
				})
			}
		}
	}
}

// TestStreamerVsLinearPerformance shows how the streamer fares against linear processing of the same events, given marginal network delays.
func TestStreamerVsLinearPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance comparison in short mode")
	}

	testCases := []struct {
		itemCount    int
		delay        time.Duration
		workerCount  int
		expectFaster bool // Whether streamer should be faster than linear
	}{
		{10, 1 * time.Millisecond, 1, false},   // Single worker should be similar to linear
		{10, 1 * time.Millisecond, 4, true},    // Multiple workers should be faster
		{50, 5 * time.Millisecond, 8, true},    // More items + workers = faster
		{100, 10 * time.Millisecond, 16, true}, // High concurrency should win
		{100, 10 * time.Millisecond, 24, true},
		{100, 10 * time.Millisecond, 36, true},
		{100, 10 * time.Millisecond, runtime.NumCPU(), true},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("items-%d/delay-%dms/workers-%d", tc.itemCount, tc.delay.Milliseconds(), tc.workerCount), func(t *testing.T) {

			// Measure linear performance
			linearStart := time.Now()
			for j := 0; j < tc.itemCount; j++ {
				time.Sleep(tc.delay) // Simulate I/O work
				_ = fmt.Sprintf("result-%d", j)
			}
			linearDuration := time.Since(linearStart)

			// Measure streamer performance
			streamer, err := NewStreamer(NewStreamerParams[int, string]{
				WorkerCount: tc.workerCount,
				Work: func(ctx context.Context, input int) (string, error) {
					time.Sleep(tc.delay) // Same I/O simulation
					return fmt.Sprintf("result-%d", input), nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			streamerStart := time.Now()

			ctx := context.Background()
			inputChan := make(chan int, tc.itemCount)

			// Send all items
			go func() {
				defer close(inputChan)
				for j := 0; j < tc.itemCount; j++ {
					inputChan <- j
				}
			}()

			results, errs, err := streamer.Stream(ctx, inputChan)
			if err != nil {
				t.Fatal(err)
			}

			// Consume all results
			resultCount := 0
			errorCount := 0

			done := make(chan struct{})
			go func() {
				for range errs {
					errorCount++
				}
				done <- struct{}{}
			}()

			for range results {
				resultCount++
			}
			<-done

			streamerDuration := time.Since(streamerStart)

			// Verify correctness first
			if resultCount != tc.itemCount {
				t.Errorf("Expected %d results, got %d", tc.itemCount, resultCount)
			}
			if errorCount > 0 {
				t.Errorf("Got %d unexpected errors", errorCount)
			}

			// Performance comparison
			speedup := float64(linearDuration) / float64(streamerDuration)

			t.Logf("Linear duration: %v", linearDuration)
			t.Logf("Streamer duration: %v", streamerDuration)
			t.Logf("Speedup: %.2fx", speedup)

			if tc.expectFaster && speedup < 1.1 { // At least 10% faster
				t.Errorf("Expected streamer to be faster than linear, but got %.2fx speedup", speedup)
			} else if !tc.expectFaster && speedup > 1.5 { // Unexpected large speedup
				t.Logf("Unexpectedly good speedup of %.2fx for single worker", speedup)
			}

			// Calculate theoretical maximum speedup
			theoreticalMax := float64(tc.workerCount)
			efficiency := speedup / theoreticalMax * 100
			t.Logf("Parallel efficiency: %.1f%% (%.2f/%d workers)", efficiency, speedup, tc.workerCount)
		})
	}
}
