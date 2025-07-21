package internal

import (
	"context"
	"errors"
	"sync"
)

type FanInParams[K any] struct {
	InputChannels []<-chan K
	Quit          <-chan int
}

// FanIn takes a series of channels and returns a single channel that centralizes their outputs.
func FanIn[K any](ctx context.Context, params FanInParams[K]) (chan K, error) {

	if len(params.InputChannels) <= 0 {
		return nil, errors.New("input channels must be greater than zero")
	}

	wg := sync.WaitGroup{}

	wg.Add(len(params.InputChannels)) // Add one to waitgroup for each channel

	output := make(chan K, 100) // Output is the result of consuming one "K" from the channel

	for _, c := range params.InputChannels { // Start a goroutine for every channel
		go func(ctx context.Context, channel <-chan K) {
			defer wg.Done()
			for i := range channel {
				select {
				case output <- i: // FORWARD MSG
				case <-params.Quit:
					return
				case <-ctx.Done():
					return
				}
			}
		}(ctx, c)
	}

	// Close output channel when all goroutines are done
	go func() {
		wg.Wait()
		close(output)
	}()

	return output, nil
}
