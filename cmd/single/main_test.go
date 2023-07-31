package main

import (
	"context"
	"go.uber.org/goleak"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// TestLeaks tests that there are no goroutine leaks after starting and stopping the server.
// We should likely do some more operations here, but this is a start.
func TestLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := realMain(ctx)
		if err != nil {
			panic("realMain error" + err.Error())
		}
	}()

	time.Sleep(time.Millisecond * 100)
	cancel()
	wg.Wait()
}
