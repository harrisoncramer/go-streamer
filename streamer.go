package streamer

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/harrisoncramer/streamer/internal"
)

// Streamer helps process items of type T in parallel, using a worker function that returns results of type K.
// It's useful when you need to process a stream of data concurrently and collect results or errors.
// Unlike the errgroup package, it handles multiple workers for you and streams the results.
// You can chain together the output of one streamer to another streamer in order to build pipelines for processing data.
type Streamer[T any, K any] struct {
	workerCount   int
	quit          <-chan int
	work          func(context.Context, T) (K, error)
	isProcessing  bool
	mu            sync.RWMutex
	wg            *sync.WaitGroup
	workerTimeout *time.Duration
}

type NewStreamerParams[T any, K any] struct {
	// The max number of concurrent workers that should be processing data at any given moment.
	WorkerCount int
	// The quit channel, if supplied, can be used as an interrupt to all workers.
	Quit <-chan int
	// The work function works on the given resource of type T, and returns K and and error as a result, which are streamed to the output and error channels. This is the business logic of the streamer.
	Work func(context.Context, T) (K, error)
	// The timeout for how long the streamer can take overall to process all of the data. Leave empty for no timeout.
	// When the timeout triggers, any subworkers will exit immediately.
	StreamerTimeout *time.Duration
	// The timeout for how long a worker should take to process the data. Leave empty for no timeout.
	// When the timeout triggers, the worker will return an error and the zero value return type immediately.
	WorkerTimeout *time.Duration
}

// NewStreamer returns an instance of a streamer capable of processing items of type T in parallel, and streaming the results to the channels
func NewStreamer[T any, K any](params NewStreamerParams[T, K]) (*Streamer[T, K], error) {

	if params.WorkerCount <= 0 {
		return nil, errors.New("worker count must be greater than zero")
	}

	if params.Work == nil {
		return nil, errors.New("work function must be defined")
	}

	return &Streamer[T, K]{
		workerCount:   params.WorkerCount,
		quit:          params.Quit,
		work:          params.Work,
		workerTimeout: params.WorkerTimeout,
		wg:            &sync.WaitGroup{},
	}, nil
}

// Stream pipes the inputs provided through the streamer's work function and returns result and error channels.
func (p *Streamer[T, K]) Stream(ctx context.Context, inputChan <-chan T) (<-chan K, <-chan error, error) {

	p.mu.Lock()
	if p.isProcessing {
		p.mu.Unlock()
		return nil, nil, errors.New("processor is already working")
	}
	p.isProcessing = true
	p.mu.Unlock()

	// Use FanOut to distribute work
	workerChannels, err := internal.FanOut(ctx, internal.FanOutParams[T]{
		Input:       inputChan,
		WorkerCount: p.workerCount,
		Quit:        p.quit,
	})
	if err != nil {
		return nil, nil, err
	}

	// Create result channels for each worker
	var outputChannels []<-chan K
	errorChan := make(chan error, p.workerCount*10) // Buffer error channel, to reduce slowdown

	for i, workerChannel := range workerChannels {
		p.wg.Add(1)

		// For each worker channel, create a result channel
		outputChan := make(chan K)
		outputChannels = append(outputChannels, outputChan)

		// Spawn a goroutine for each read off of the fanned out workers.
		// Take the value read, and pass it to the work function. Send any errors to the error channel and any outputs to the output channel.
		go func(workerID int, inputs <-chan T, output chan<- K) {
			defer close(output)
			defer p.wg.Done()
			for input := range inputs {
				workCtx := ctx
				var cancel context.CancelFunc
				if p.workerTimeout != nil {
					workCtx, cancel = context.WithTimeout(ctx, *p.workerTimeout)
				}
				res, err := p.work(workCtx, input)

				if cancel != nil {
					cancel()
				}

				if err != nil {
					errorChan <- err
				} else {
					output <- res
				}
			}
		}(i, workerChannel, outputChan)
	}

	// Close the error channel and reset the streamer's state when workers finish
	go func() {
		p.wg.Wait()
		close(errorChan)
		p.mu.Lock()
		p.wg = nil
		p.isProcessing = false
		p.mu.Unlock()

	}()

	// Use FanIn to aggregate results from all the workers, and return the single channel
	results, err := internal.FanIn(ctx, internal.FanInParams[K]{
		InputChannels: outputChannels,
		Quit:          p.quit,
	})

	return results, errorChan, err
}

// Flush can be used ensure that all of the values supplied to the streamer's input channel have been worked.
func (p *Streamer[T, K]) Flush() {
	p.mu.RLock()
	wg := p.wg
	p.mu.RUnlock()

	if wg != nil {
		wg.Wait()
	}
}
