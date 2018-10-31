package supervised

import (
	"context"
	"github.com/orbs-network/orbs-network-go/instrumentation/log"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type report struct {
	message string
	fields  []*log.Field
}

type collector struct {
	errors chan report
}

func (c *collector) Error(message string, fields ...*log.Field) {
	c.errors <- report{message, fields}
}

func mockLogger() *collector {
	c := &collector{errors: make(chan report)}
	return c
}

func localFunctionThatPanics() {
	panic("foo")
}

func TestShortLived_ReportsOnPanic(t *testing.T) {
	logger := mockLogger()

	require.NotPanicsf(t, func() {
		ShortLived(logger, localFunctionThatPanics)
	}, "ShortLived panicked unexpectedly")

	report := <-logger.errors
	require.Equal(t, report.message, "recovered panic")
	require.Len(t, report.fields, 2, "expected log to contain both error and stack trace")

	errorField := report.fields[0]
	stackTraceField := report.fields[1]
	require.Contains(t, errorField.Value(), "foo")
	require.Equal(t, stackTraceField.Key, "stack-trace")
	require.Equal(t, stackTraceField.Key, "stack-trace")
	require.Contains(t, stackTraceField.Value(), "localFunctionThatPanics")
}

func TestLongLived_ReportsOnPanicAndRestarts(t *testing.T) {
	numOfIterations := 10

	logger := mockLogger()
	ctx, cancel := context.WithCancel(context.Background())

	count := 0

	require.NotPanicsf(t, func() {
		LongLived(ctx, logger, func() {
			count++
			if count > numOfIterations {
				cancel()
			}
			panic("foo")
		})
	}, "LongLived panicked unexpectedly")

	for i := 0; i < numOfIterations; i++ {
		select {
		case report := <-logger.errors:
			require.Equal(t, report.message, "recovered panic")
		case <-time.After(1 * time.Second):
			require.Fail(t, "long living goroutine didn't restart")
		}
	}

}
