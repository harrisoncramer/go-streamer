package internal

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Transaction struct {
	ID       string
	Amount   float64
	UserID   string
	Type     string
	Currency string
}

type Account struct {
	ID      string
	Balance float64
	UserID  string
	Type    string
}

type User struct {
	ID    string
	Name  string
	Email string
}

func TestFanIn(t *testing.T) {
	tests := []struct {
		name          string
		inputChannels [][]any
		expectedCount int
		expectedItems []any
		useCancel     bool
		cancelAfter   time.Duration
		timeout       time.Duration
	}{
		{
			name: "multiple transaction channels",
			inputChannels: [][]any{
				{
					Transaction{ID: "tx1", Amount: 100.50, UserID: "user1", Type: "deposit", Currency: "USD"},
					Transaction{ID: "tx2", Amount: 250.75, UserID: "user2", Type: "withdrawal", Currency: "USD"},
				},
				{
					Transaction{ID: "tx3", Amount: 500.00, UserID: "user3", Type: "transfer", Currency: "EUR"},
					Transaction{ID: "tx4", Amount: 75.25, UserID: "user1", Type: "deposit", Currency: "GBP"},
				},
				{
					Transaction{ID: "tx5", Amount: 1000.00, UserID: "user4", Type: "payment", Currency: "USD"},
				},
			},
			expectedCount: 5,
			timeout:       2 * time.Second,
		},
		{
			name: "multiple account channels",
			inputChannels: [][]any{
				{
					Account{ID: "acc1", Balance: 1500.00, UserID: "user1", Type: "checking"},
					Account{ID: "acc2", Balance: 2500.50, UserID: "user2", Type: "savings"},
				},
				{
					Account{ID: "acc3", Balance: 750.25, UserID: "user3", Type: "investment"},
				},
			},
			expectedCount: 3,
			timeout:       2 * time.Second,
		},
		{
			name: "mixed user data channels",
			inputChannels: [][]any{
				{
					User{ID: "user1", Name: "Alice Johnson", Email: "alice@example.com"},
					User{ID: "user2", Name: "Bob Smith", Email: "bob@example.com"},
				},
				{
					User{ID: "user3", Name: "Carol Davis", Email: "carol@example.com"},
					User{ID: "user4", Name: "David Wilson", Email: "david@example.com"},
				},
			},
			expectedCount: 4,
			timeout:       2 * time.Second,
		},
		{
			name:          "empty channels",
			inputChannels: [][]any{{}, {}, {}},
			expectedCount: 0,
			timeout:       1 * time.Second,
		},
		{
			name: "single channel with transactions",
			inputChannels: [][]any{
				{
					Transaction{ID: "tx1", Amount: 100.00, UserID: "user1", Type: "deposit", Currency: "USD"},
					Transaction{ID: "tx2", Amount: 200.00, UserID: "user2", Type: "withdrawal", Currency: "USD"},
				},
			},
			expectedCount: 2,
			timeout:       1 * time.Second,
		},
		{
			name:          "context cancellation with delayed input",
			inputChannels: [][]any{{Transaction{ID: "tx1", Amount: 100.00, UserID: "user1", Type: "deposit", Currency: "USD"}}},
			expectedCount: 1,
			useCancel:     true,
			cancelAfter:   10 * time.Millisecond,
			timeout:       1 * time.Second,
		},
		{
			name: "large volume transaction processing",
			inputChannels: func() [][]any {
				channels := make([][]any, 3)
				for i := range 3 {
					channel := make([]any, 100)
					for j := range 100 {
						channel[j] = Transaction{
							ID:       fmt.Sprintf("tx%d_%d", i, j),
							Amount:   float64(j * 10),
							UserID:   fmt.Sprintf("user%d", j%10),
							Type:     "automated",
							Currency: "USD",
						}
					}
					channels[i] = channel
				}
				return channels
			}(),
			expectedCount: 300,
			timeout:       5 * time.Second,
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

			// Create input channels, send each set of data from our test suite
			var inputChans []<-chan any
			for _, items := range tt.inputChannels {
				ch := make(chan any, len(items))
				for _, item := range items {
					ch <- item
				}
				close(ch)
				inputChans = append(inputChans, ch)
			}

			// Start cancel goroutine if needed
			if tt.useCancel {
				go func() {
					time.Sleep(tt.cancelAfter)
					cancel()
				}()
			}

			// Call FanIn
			output, _ := FanIn(ctx, FanInParams[any]{
				InputChannels: inputChans,
				Quit:          nil,
			})

			// Collect results
			results := collectAllResults(t, output, tt.timeout)

			// Assertions
			if tt.useCancel {
				// When context is cancelled, we might get fewer items
				assert.LessOrEqual(t, len(results), tt.expectedCount, "Should not exceed expected count when cancelled")
			} else {
				assert.Equal(t, tt.expectedCount, len(results), "Should receive all expected items")
			}

			// Verify channel is closed
			select {
			case _, ok := <-output:
				assert.False(t, ok, "Output channel should be closed")
			default:
				t.Error("Output channel should be closed and readable")
			}
		})
	}
}

func collectAllResults(t *testing.T, output <-chan any, timeout time.Duration) []any {
	var results []any
	timeoutCh := time.After(timeout)

	for {
		select {
		case item, ok := <-output:
			if !ok {
				return results
			}
			results = append(results, item)
		case <-timeoutCh:
			t.Fatal("Test timed out")
		}
	}
}

func TestFanInWithSpecificTypes(t *testing.T) {
	t.Run("string messages", func(t *testing.T) {
		ctx := context.Background()

		ch1 := make(chan string, 2)
		ch1 <- "Account balance updated for user123"
		ch1 <- "Transaction processed: $500.00"
		close(ch1)

		ch2 := make(chan string, 2)
		ch2 <- "KYC verification completed for user456"
		ch2 <- "Fraud alert: suspicious transaction detected"
		close(ch2)

		output, _ := FanIn(ctx, FanInParams[string]{
			InputChannels: []<-chan string{ch1, ch2},
			Quit:          nil,
		})

		messages := collectStringResults(t, output, 2*time.Second)

		require.Equal(t, 4, len(messages))
		sort.Strings(messages) // Sort for consistent comparison

		expected := []string{
			"Account balance updated for user123",
			"Fraud alert: suspicious transaction detected",
			"KYC verification completed for user456",
			"Transaction processed: $500.00",
		}
		sort.Strings(expected)

		assert.Equal(t, expected, messages)
	})
}

func collectStringResults(t *testing.T, output <-chan string, timeout time.Duration) []string {
	var messages []string
	timeoutCh := time.After(timeout)

	for {
		select {
		case msg, ok := <-output:
			if !ok {
				return messages
			}
			messages = append(messages, msg)
		case <-timeoutCh:
			t.Fatal("Test timed out")
		}
	}
}

func TestFanInEdgeCases(t *testing.T) {
	t.Run("no input channels", func(t *testing.T) {
		output, err := FanIn(context.Background(), FanInParams[int]{
			InputChannels: []<-chan int{},
			Quit:          nil,
		})

		require.Error(t, err)
		require.Nil(t, output)
	})
}
