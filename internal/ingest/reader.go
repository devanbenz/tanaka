// Package ingest reads raw technical content and structures it into sections.
package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// httpClient is a package-level client with a 30-second timeout.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// RawSource is unstructured ingested content plus its origin.
type RawSource struct {
	Origin string
	Bytes  []byte
}

// Read loads content from a file path, an http(s) URL, or stdin ("-").
func Read(ctx context.Context, arg string, stdin io.Reader) (*RawSource, error) {
	switch {
	case arg == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return &RawSource{Origin: "stdin", Bytes: b}, nil
	case strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, arg, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", arg, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: status %d", arg, resp.StatusCode)
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		return &RawSource{Origin: arg, Bytes: b}, nil
	default:
		b, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", arg, err)
		}
		return &RawSource{Origin: arg, Bytes: b}, nil
	}
}
