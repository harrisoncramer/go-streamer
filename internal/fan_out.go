package internal

import (
	"context"
	"errors"
)

type FanOutParams[K any] struct {
	Quit        <-chan int
	Input       <-chan K
	WorkerCount int
}

// FanOut takes a single channel and distributes its values to "n" workers in round-robin fashion.
func FanOut[K any](ctx context.Context, params FanOutParams[K]) ([]<-chan K, error) {
	if params.WorkerCount <= 0 {
		return nil, errors.New("worker count must be greater than zero")
	}
	if params.Input == nil {
		return nil, errors.New("input channel cannot be nil")
	}

	outputs := make([]chan K, params.WorkerCount)
	readOnlyOutputs := make([]<-chan K, params.WorkerCount) // Return read-only channels to consumers

	for i := range params.WorkerCount { // Create one channel for each worker
		outputs[i] = make(chan K)
		readOnlyOutputs[i] = outputs[i]
	}

	// Start distributor goroutine.
	// We return immediately a single channel after this, which the distributor is responsible
	// for sending input on from all of the workers.

	go func(ctx context.Context) {

		// Don't forget to close all output channels when done
		defer func() {
			for _, ch := range outputs {
				close(ch)
			}
		}()

		workerIndex := 0 // Round-robin index for distributing work

		for {
			select {
			case item, ok := <-params.Input:
				if !ok {
					return
				}
				select {
				case outputs[workerIndex] <- item: // Distribute to worker on the worker index, and move to next worker
					workerIndex = (workerIndex + 1) % params.WorkerCount
				case <-ctx.Done():
					return
				case <-params.Quit:
					return
				}
			case <-ctx.Done():
				return
			case <-params.Quit:
				return
			}
		}

	}(ctx)

	return readOnlyOutputs, nil
}
