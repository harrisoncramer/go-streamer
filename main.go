package main

import (
	"context"
	"fmt"

	"github.com/harrisoncramer/streamer"
)

func main() {
	// Create a streamer that squares integers
	streamer, err := streamer.NewStreamer(streamer.NewStreamerParams[int, int]{
		WorkerCount: 3,
		Work: func(ctx context.Context, n int) (int, error) {
			return n * n, nil
		},
	})
	if err != nil {
		panic(err)
	}

	// Create input channel
	input := make(chan int, 5)
	input <- 1
	input <- 2
	input <- 3
	input <- 4
	input <- 5
	close(input)

	// Process the data
	results, errors, err := streamer.Stream(context.Background(), input)
	if err != nil {
		panic(err)
	}

	// Collect results
	for result := range results {
		fmt.Printf("Result: %d\n", result)
	}

	// Check for errors
	for err := range errors {
		fmt.Printf("Error: %v\n", err)
	}
}
