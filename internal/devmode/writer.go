package devmode

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/alternayte/restitch-gateway/internal/upstream"
)

const (
	ColorCyan    = "\033[36m"
	ColorMagenta = "\033[35m"
	ColorYellow  = "\033[33m"
	ColorReset   = "\033[0m"
)

type PrefixWriter struct {
	mu        sync.Mutex
	dest      io.Writer
	prefix    string
	colorCode string
	noColor   bool
}

func NewPrefixWriter(dest io.Writer, prefix, colorCode string) *PrefixWriter {
	_, noColor := os.LookupEnv("NO_COLOR")
	return &PrefixWriter{
		dest:      dest,
		prefix:    prefix,
		colorCode: colorCode,
		noColor:   noColor,
	}
}

func (pw *PrefixWriter) Write(p []byte) (int, error) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	var tag string
	if pw.noColor {
		tag = fmt.Sprintf("[%s]", pw.prefix)
	} else {
		tag = fmt.Sprintf("%s[%s]%s", pw.colorCode, pw.prefix, ColorReset)
	}

	scanner := bufio.NewScanner(bytes.NewReader(p))
	for scanner.Scan() {
		fmt.Fprintf(pw.dest, "%s %s\n", tag, scanner.Text())
	}
	return len(p), scanner.Err()
}

func WaitForHealth(ctx context.Context, url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 2 * time.Second}

	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 200 * time.Millisecond
	bo.MaxInterval = 2 * time.Second

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return struct{}{}, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return struct{}{}, err
		}
		defer upstream.DrainAndClose(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return struct{}{}, fmt.Errorf("health check returned %d", resp.StatusCode)
		}
		return struct{}{}, nil
	}, backoff.WithBackOff(bo))
	return err
}
