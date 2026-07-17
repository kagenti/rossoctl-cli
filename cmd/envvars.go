package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kagenti/rossoctl-cli/internal/apiclient"
)

// fetchEnvVars GETs envURL and parses its body as newline-separated key=value
// pairs into a slice of EnvVars. Blank lines and lines beginning with '#' are
// ignored. An empty envURL returns nil (no env vars).
//
// The current context's bearer token is sent only when envURL is on the same
// host as the API server, so a public env document (e.g. on GitHub) is fetched
// anonymously — sending the API token to a foreign host both leaks it and, for
// hosts like raw.githubusercontent.com, causes an unrelated 404.
func fetchEnvVars(ctx context.Context, cmd *cobra.Command, envURL string) ([]apiclient.EnvVar, error) {
	if envURL == "" {
		return nil, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, envURL, nil)
	if err != nil {
		return nil, err
	}
	if server, token, terr := resolveServer(); terr == nil && token != "" && sameHost(server, envURL) {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	if verbose {
		fmt.Fprintf(cmd.ErrOrStderr(), "GET %s (env vars)\n", envURL)
	}

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching env vars from %s: %w", envURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetching env vars from %s: HTTP %d", envURL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading env vars from %s: %w", envURL, err)
	}

	return parseEnvVars(string(body))
}

// sameHost reports whether two URLs have the same host (including port). Used
// to decide whether the API bearer token may be sent to the env-vars URL.
func sameHost(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return ua.Host != "" && ua.Host == ub.Host
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
