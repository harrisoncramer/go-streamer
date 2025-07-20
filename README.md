# streamer

This module implements a generic streamer package for Go.

The streamer is capable of accepting inputs from an input worker and distributing the work in round-robin fashion to a set of worker routines, and then streaming the results to an output channel.

## Features

- **Generic Types**: Works with any input type `T` and output type `K`
- **Concurrent Processing**: Configurable number of worker goroutines
- **Worker Timeouts**: Set timeouts per worker operation to prevent hanging
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
    "fmt"
    "time"
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
```

## Advanced Usage

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
quit := make(chan int)
streamer, err := streamer.NewStreamer(streamer.NewStreamerParams[int, string]{
    WorkerCount: 2,
    Quit:        quit,
    Work: func(ctx context.Context, n int) (string, error) {
        return fmt.Sprintf("item-%d", n), nil
    },
})

// Later, signal all workers to quit
close(quit)
```

### Building Pipelines

```go
// First stage: convert strings to integers
stage1, _ := streamer.NewStreamer(streamer.NewStreamerParams[string, int]{
    WorkerCount: 2,
    Work: func(ctx context.Context, s string) (int, error) {
        return strconv.Atoi(s)
    },
})

// Second stage: square the integers  
stage2, _ := streamer.NewStreamer(streamer.NewStreamerParams[int, int]{
    WorkerCount: 3,
    Work: func(ctx context.Context, n int) (int, error) {
        return n * n, nil
    },
})

// Chain them together
input := make(chan string)
results1, errors1, _ := stage1.Stream(ctx, input)
results2, errors2, _ := stage2.Stream(ctx, results1)

// Handle final results and errors from both stages
```

## Requirements

- Go 1.21+ (for generic type parameters)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality  
4. Ensure all tests pass
5. Submit a pull request
