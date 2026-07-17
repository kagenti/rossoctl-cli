package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
)

// fetchEnvVars GETs url and parses its body as newline-separated key=value
// pairs into a slice of EnvVars. Blank lines and lines beginning with '#' are
// ignored. The current context's bearer token is sent, in case the URL is on
// the same protected server. An empty url returns nil (no env vars).
func fetchEnvVars(ctx context.Context, cmd *cobra.Command, url string) ([]apiclient.EnvVar, error) {
	if url == "" {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if _, token, terr := resolveServer(); terr == nil && token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if verbose {
		fmt.Fprintf(cmd.ErrOrStderr(), "GET %s (env vars)\n", url)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching env vars from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetching env vars from %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading env vars from %s: %w", url, err)
	}

	return parseEnvVars(string(body))
}

// parseEnvVars parses newline-separated key=value pairs. Surrounding
// whitespace on each line is trimmed; blank lines and '#' comments are
// skipped. A line without '=' or with an empty key is an error.
func parseEnvVars(body string) ([]apiclient.EnvVar, error) {
	var out []apiclient.EnvVar
	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid env var on line %d: %q (expected key=value)", i+1, raw)
		}
		out = append(out, apiclient.EnvVar{Name: key, Value: strings.TrimSpace(value)})
	}
	return out, nil
}
