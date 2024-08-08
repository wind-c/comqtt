package rest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	HttpGet    = "GET"
	HttpPost   = "POST"
	HttpDelete = "DELETE"
	Timeout    = 3 * time.Second
)

func fetch(ctx context.Context, method string, url string, data []byte) result {
	rs := result{Url: url}
	var req *http.Request
	var err error
	var body io.Reader
	if data != nil {
		body = bytes.NewBuffer(data)
	}
	if ctx != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, body)
	} else {
		req, err = http.NewRequest(method, url, body)
	}
	if err != nil {
		rs.Err = err.Error()
		return rs
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); errors.Is(ctxErr, context.DeadlineExceeded) {
			rs.Err = "Request timeout"
		} else {
			rs.Err = err.Error()
		}
		return rs
	}
	defer resp.Body.Close()
	if data, err := io.ReadAll(resp.Body); err != nil {
		rs.Err = err.Error()
	} else {
		if resp.StatusCode == http.StatusOK {
			rs.Data = string(data)
		} else {
			rs.Err = string(data)
		}
	}
	return rs
}

func fetchM(method string, urls []string, body []byte) []result {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	var wg sync.WaitGroup
	ch := make(chan result, len(urls))

	wg.Add(len(urls))
	for _, url := range urls {
		go func(url string) {
			defer wg.Done()
			ch <- fetch(ctx, method, url, body)
		}(url)
	}

	wg.Wait()
	close(ch)
	results := make([]result, len(urls))
	index := 0
	for rs := range ch {
		results[index] = rs
		index++
	}
	return results
}
