# streamer

This Go module provides an abstraction for streaming data through user-defined "work" functions and aggregating the results, in multiple concurrent goroutines. 

It hides the complexity of distributing the work, aggregating results, handling timeouts, and collecting errors, and lets users focus on their application business logic. It's well-suited for optimizing in-memory processing of IO-bound operations (think HTTP requests, database reads, etc) where persistence is unimportant, as the module has no persistence layer. 

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

## Tests

Run tests:

```bash
go test ./...
```

Run benchmarks:

```bash
go -test.bench Streamer_IOBound
```

Results on an M4 Mac show excellent parallelization for IO-bound workflows. Processing time decreases proportionally with worker count:

- **10 items, 1ms delay**: 1 worker (11.3ms) → 16 workers (1.1ms) = **10x speedup**
- **100 items, 10ms delay**: 1 worker (1,090ms) → 16 workers (76.5ms) = **14x speedup**

```
goos: darwin
goarch: arm64
pkg: github.com/harrisoncramer/streamer
cpu: Apple M4 Pro
BenchmarkStreamer_IOBound/items-10/delay-1ms/workers-1-14                    100          11322906 ns/op            7728 B/op         67 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-1ms/workers-2-14                    211           5670585 ns/op           10079 B/op         75 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-1ms/workers-4-14                    352           3402951 ns/op           15055 B/op         90 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-1ms/workers-8-14                    526           2272449 ns/op           24705 B/op        119 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-1ms/workers-16-14                  1045           1149089 ns/op           44059 B/op        176 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-1ms/workers-14-14                  1042           1149217 ns/op           39174 B/op        162 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-5ms/workers-1-14                     20          56170606 ns/op            7669 B/op         67 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-5ms/workers-2-14                     43          27999681 ns/op           10041 B/op         75 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-5ms/workers-4-14                     73          16737883 ns/op           14931 B/op         90 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-5ms/workers-8-14                    100          11178828 ns/op           24674 B/op        119 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-5ms/workers-16-14                   212           5629430 ns/op           44096 B/op        177 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-5ms/workers-14-14                   212           5603733 ns/op           39170 B/op        162 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-10ms/workers-1-14                    10         109382429 ns/op            7778 B/op         67 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-10ms/workers-2-14                    21          54530714 ns/op           10080 B/op         75 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-10ms/workers-4-14                    36          32791212 ns/op           14932 B/op         90 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-10ms/workers-8-14                    55          21939009 ns/op           24643 B/op        119 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-10ms/workers-16-14                  100          10946720 ns/op           44068 B/op        177 allocs/op
BenchmarkStreamer_IOBound/items-10/delay-10ms/workers-14-14                  100          10910312 ns/op           39212 B/op        163 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-1ms/workers-1-14                     20          56571333 ns/op           18552 B/op        227 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-1ms/workers-2-14                     42          28314992 ns/op           20908 B/op        235 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-1ms/workers-4-14                     80          14733299 ns/op           25798 B/op        250 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-1ms/workers-8-14                    150           7942072 ns/op           35599 B/op        279 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-1ms/workers-16-14                   262           4547488 ns/op           54916 B/op        336 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-1ms/workers-14-14                   262           4549441 ns/op           50041 B/op        322 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-5ms/workers-1-14                      4         280593896 ns/op           18986 B/op        227 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-5ms/workers-2-14                      8         139862688 ns/op           21207 B/op        236 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-5ms/workers-4-14                     15          72574767 ns/op           26057 B/op        251 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-5ms/workers-8-14                     30          39155733 ns/op           35613 B/op        279 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-5ms/workers-16-14                    54          22394231 ns/op           54872 B/op        336 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-5ms/workers-14-14                    54          22464897 ns/op           50052 B/op        322 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-10ms/workers-1-14                     2         547526562 ns/op           19532 B/op        228 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-10ms/workers-2-14                     4         272946927 ns/op           21678 B/op        239 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-10ms/workers-4-14                     8         142339932 ns/op           26025 B/op        250 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-10ms/workers-8-14                    15          76511586 ns/op           35642 B/op        279 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-10ms/workers-16-14                   27          43725130 ns/op           55238 B/op        339 allocs/op
BenchmarkStreamer_IOBound/items-50/delay-10ms/workers-14-14                   27          43756745 ns/op           50085 B/op        322 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-1ms/workers-1-14                     9         113117759 ns/op           32394 B/op        427 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-1ms/workers-2-14                    20          56597881 ns/op           34677 B/op        435 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-1ms/workers-4-14                    42          28319260 ns/op           39515 B/op        450 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-1ms/workers-8-14                    80          14731343 ns/op           49307 B/op        479 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-1ms/workers-16-14                  150           7952676 ns/op           68684 B/op        537 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-1ms/workers-14-14                  132           9082375 ns/op           63798 B/op        523 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-5ms/workers-1-14                     2         561091125 ns/op           33244 B/op        428 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-5ms/workers-2-14                     4         280003948 ns/op           35438 B/op        439 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-5ms/workers-4-14                     8         139966812 ns/op           39775 B/op        450 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-5ms/workers-8-14                    15          72508664 ns/op           49692 B/op        482 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-5ms/workers-16-14                   30          39182549 ns/op           68870 B/op        539 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-5ms/workers-14-14                   26          44703816 ns/op           63836 B/op        523 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-10ms/workers-1-14                    1        1090081000 ns/op           34336 B/op        430 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-10ms/workers-2-14                    2         544774021 ns/op           36092 B/op        442 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-10ms/workers-4-14                    4         272582677 ns/op           40010 B/op        450 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-10ms/workers-8-14                    8         141910958 ns/op           49463 B/op        479 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-10ms/workers-16-14                  15          76578381 ns/op           68785 B/op        537 allocs/op
BenchmarkStreamer_IOBound/items-100/delay-10ms/workers-14-14                  13          86996176 ns/op           64033 B/op        524 allocs/op
```
