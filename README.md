# streamer

This Go module provides an abstraction for streaming data through user-defined "work" functions and aggregating the results, in multiple concurrent goroutines. 

It hides the complexity of distributing the work, aggregating results, handling timeouts, and collecting errors, and lets users focus on their application business logic. It's well-suited for optimizing in-memory processing of data, where persistence is unimportant, as the module has no persistence layer.j

This module has no external dependencies.

https://github.com/user-attachments/assets/e441ca63-b77e-469a-b876-5a565692e426

## Features

- **Generic Types**: Works with any input type `T` and output type `K`
- **Concurrent Processing**: Configurable number of worker goroutines
- **Worker Timeouts**: Set timeouts per worker operation to prevent hanging
- **Retry Support**: Configurable backoffs and retry mechanism for failing workers
- **Error Handling**: Separate channels for results and errors
- **Graceful Shutdown**: Support for quit channels and context cancellation  

## Installation

```bash
go get github.com/harrisoncramer/streamer
```

## Quick Start

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"
    "log"

	"github.com/harrisoncramer/streamer"
)

func main() {
	// Create a streamer that squares integers
	streamer, err := streamer.NewStreamer(streamer.NewStreamerParams[int, int]{
		WorkerCount: 1,
		Work: streamer.CreateRetryableWorkFunc(
			func(ctx context.Context, n int) (int, error) {
				rand := rand.Intn(10)
				if rand > 5 {
					return 0, errors.New("some random error")
				}
				return n * n, nil
			},
			streamer.WithFixedBackoff(200*time.Millisecond),
			streamer.WithRetryCondition(func(error) bool { return true }),
			streamer.WithRetries(5),
		),
	})

	if err != nil {
		panic(err)
	}

	// Create input channel
	input := make(chan int, 10_000)
	for i := range 10_000 {
		input <- i
	}
	close(input)

	// Process the data
	results, errors, err := streamer.Stream(context.Background(), input)
	if err != nil {
        log.Fatal(err)
	}

	go func() {
		for result := range results {
			fmt.Printf("Result: %d\n", result)
		}
	}()

	go func() {
		for err := range errors {
			fmt.Printf("Error: %v\n", err)
		}
	}()

	streamer.Flush()
}
```

### With Worker Timeouts

```go
timeout := 1 * time.Second
streamer, err := streamer.NewStreamer(streamer.NewStreamerParams[string, string]{
    WorkerCount:   4,
    TimeoutWorker: &timeout,
    Work: func(ctx context.Context, data string) (string, error) {
        select {
        case <-time.After(2 * time.Second):
            return "processed: " + data, nil // This will not select, the timeout will happen first.
        case <-ctx.Done():
            return "", ctx.Err()
        }
    },
})
```

### With Quit Channel

```go
quit := make(chan int) // Signaling on this channel will kill workers
streamer, err := streamer.NewStreamer(streamer.NewStreamerParams[int, string]{
    WorkerCount: 2,
    Quit:        quit,
    Work: func(ctx context.Context, n int) (string, error) {
        return fmt.Sprintf("item-%d", n), nil
    },
})
```

### Building Pipelines

```go
stage1, _ := streamer.NewStreamer(streamer.NewStreamerParams[string, int]{
    WorkerCount: 2,
    Work: func(ctx context.Context, s string) (int, error) {
        return strconv.Atoi(s)
    },
})

stage2, _ := streamer.NewStreamer(streamer.NewStreamerParams[int, int]{
    WorkerCount: 3,
    Work: func(ctx context.Context, n int) (int, error) {
        return n * n, nil
    },
})

input := make(chan string)
results1, errors1, _ := stage1.Stream(ctx, input)
results2, errors2, _ := stage2.Stream(ctx, results1)
```

### Adding Retries

```go
streamer, _ := NewStreamer(NewStreamerParams[int, string]{
    WorkerCount: 5,
    Work: CreateRetryableWorkFunc(
        originalWorkFunc,
        WithRetries(3),
        WithLinearBackoff(),
        WithRetryCondition(func(err error) bool {
            return errors.As(err, &someSentinelError)
        }),
    ),
})
```

And a more complex example:

```go
streamer, _ := NewStreamer(NewStreamerParams[int, string]{
    WorkerCount: 5,
    Work: CreateRetryableWorkFunc(
        originalWorkFunc,
        WithRetries(3),
        WithExponentialBackoff(),
        WithRetryCondition(AnyOf(
            WithRetryCondition(func(err error) bool {
                return someBusinessLogic()
            }),
            WithRetryOnErrors(ErrRateLimited, ErrServiceBusy),
        )),
    ),
})
```

## Requirements

- Go 1.21+ (for generic type parameters)
