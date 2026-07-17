// Package deviceflow implements the OAuth 2.0 Device Authorization Grant
// (RFC 8628) against a Keycloak realm.
//
// It is used by `rossoctl login` (with no --token) to obtain a bearer token
// interactively: request a device code, show the user a verification URL and
// code, then poll the token endpoint until the user authorizes.
//
// Like the other internal packages it is free of Cobra. Time is injected via
// the Sleep field so polling can be tested without real delays.
package deviceflow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a Keycloak realm's device and token endpoints.
type Client struct {
	// KeycloakURL is the Keycloak base URL, e.g. https://kc.example.com.
	KeycloakURL string
	// Realm is the Keycloak realm name.
	Realm string
	// ClientID is the public OAuth client id used for the device flow.
	ClientID string

	// HTTPClient is used for requests. If nil, a client with a sensible
	// timeout is used.
	HTTPClient *http.Client

	// Logf, if set, logs each request and its outcome (never the token).
	Logf func(format string, args ...any)

	// Sleep waits for d before the next poll. If nil, time.Sleep is used.
	// Tests inject a no-op (or a recorder) to avoid real delays.
	Sleep func(d time.Duration)
}

// DeviceAuth is the response from the device authorization endpoint.
type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// tokenResponse is the (success or error) response from the token endpoint.
type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) sleep(d time.Duration) {
	if c.Sleep != nil {
		c.Sleep(d)
		return
	}
	time.Sleep(d)
}

// endpoint builds <KeycloakURL>/realms/<Realm>/protocol/openid-connect/<name>.
func (c *Client) endpoint(name string) (string, error) {
	if c.KeycloakURL == "" {
		return "", fmt.Errorf("keycloak URL is empty")
	}
	if c.Realm == "" {
		return "", fmt.Errorf("keycloak realm is empty")
	}
	base := strings.TrimSuffix(c.KeycloakURL, "/")
	return fmt.Sprintf("%s/realms/%s/protocol/openid-connect/%s",
		base, url.PathEscape(c.Realm), name), nil
}

// postForm POSTs form as application/x-www-form-urlencoded and decodes the
// JSON body into out. It returns the HTTP status code so callers can
// distinguish pending/slow-down (400) from other failures.
func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	c.logf("POST %s", endpoint)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, fmt.Errorf("requesting %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	c.logf("POST %s -> %s", endpoint, resp.Status)

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("reading response from %s: %w", endpoint, err)
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decoding response from %s: %w", endpoint, err)
		}
	}
	return resp.StatusCode, nil
}

// RequestDeviceCode calls the device authorization endpoint and returns the
// device/user codes and verification URIs.
func (c *Client) RequestDeviceCode(ctx context.Context) (*DeviceAuth, error) {
	endpoint, err := c.endpoint("auth/device")
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("client_id", c.ClientID)

	var da DeviceAuth
	status, err := c.postForm(ctx, endpoint, form, &da)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("device authorization request failed: HTTP %d", status)
	}
	if da.DeviceCode == "" {
		return nil, fmt.Errorf("device authorization response had no device_code")
	}
	// Keycloak may omit interval; default to 5s per RFC 8628.
	if da.Interval <= 0 {
		da.Interval = 5
	}
	return &da, nil
}

// PollToken polls the token endpoint until the user authorizes (returning the
// access token), the code expires, or the user denies. It honors the server's
// polling interval and backs off on slow_down. The provided context bounds the
// overall wait (cancel/deadline stops polling).
func (c *Client) PollToken(ctx context.Context, da *DeviceAuth) (string, error) {
	endpoint, err := c.endpoint("token")
	if err != nil {
		return "", err
	}

	interval := time.Duration(da.Interval) * time.Second
	for {
		// Wait before each poll (RFC 8628 says clients wait `interval`
		// between polls; waiting first also gives the user time to act).
		c.sleep(interval)
		if err := ctx.Err(); err != nil {
			return "", err
		}

		form := url.Values{}
		form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		form.Set("device_code", da.DeviceCode)
		form.Set("client_id", c.ClientID)

		var tr tokenResponse
		status, err := c.postForm(ctx, endpoint, form, &tr)
		if err != nil {
			return "", err
		}

		if tr.AccessToken != "" {
			return tr.AccessToken, nil
		}

		switch tr.Error {
		case "authorization_pending":
			// Keep polling at the current interval.
		case "slow_down":
			// RFC 8628: increase the interval by 5 seconds.
			interval += 5 * time.Second
		case "access_denied":
			return "", fmt.Errorf("authorization denied by the user")
		case "expired_token":
			return "", fmt.Errorf("device code expired before authorization; run login again")
		case "":
			return "", fmt.Errorf("token request failed: HTTP %d", status)
		default:
			if tr.ErrorDescription != "" {
				return "", fmt.Errorf("token request failed: %s (%s)", tr.Error, tr.ErrorDescription)
			}
			return "", fmt.Errorf("token request failed: %s", tr.Error)
		}
	}
}
