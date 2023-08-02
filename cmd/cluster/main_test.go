package main

import (
	"context"
	"go.uber.org/goleak"
	"os"
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
		os.Args = []string{"binary", "-node-name", "testnode", "-members", "localhost:7946",
			"-raft-bootstrap", "true", "-storage-way", "memory"}
		err := realMain(ctx)
		if err != nil {
			panic("realMain error" + err.Error())
		}
	}()

	time.Sleep(time.Millisecond * 100)
	cancel()
	wg.Wait()
	time.Sleep(time.Millisecond * 100)
}
