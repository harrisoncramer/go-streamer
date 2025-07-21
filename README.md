# streamer

This Go module provides an abstraction for streaming data through user-defined "work" functions and aggregating the results, in multiple concurrent goroutines. 

It hides the complexity of distributing the work, aggregating results, handling timeouts, and collecting errors, and lets users focus on their application business logic. It's well-suited for optimizing in-memory processing of data, where persistence and retry mechanisms are unimportant, as the module has no persistence layer. 

This module has no external dependencies.

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

## Requirements

- Go 1.21+ (for generic type parameters)
