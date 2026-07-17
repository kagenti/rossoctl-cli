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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
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

	// NewVerifier returns a fresh PKCE code_verifier. If nil, a random one is
	// generated. Tests inject a fixed value for deterministic assertions.
	NewVerifier func() (string, error)
}

// DeviceAuth is the response from the device authorization endpoint.
type DeviceAuth struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`

	// codeVerifier is the PKCE verifier generated for this authorization; it
	// is sent with the token request. Not part of the wire response.
	codeVerifier string `json:"-"`
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

// generateVerifier returns a high-entropy PKCE code_verifier: 32 random bytes
// base64url-encoded (no padding), yielding 43 chars within the RFC 7636 range.
func generateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating PKCE verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// challengeS256 derives the PKCE code_challenge for the S256 method:
// base64url(SHA256(verifier)) with no padding.
func challengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *Client) newVerifier() (string, error) {
	if c.NewVerifier != nil {
		return c.NewVerifier()
	}
	return generateVerifier()
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
// JSON body into out. It returns the HTTP status code and the raw response
// body so callers can distinguish pending/slow-down (400) from other failures
// and surface the server's error details.
func (c *Client) postForm(ctx context.Context, endpoint string, form url.Values, out any) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	c.logf("POST %s", endpoint)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("requesting %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	c.logf("POST %s -> %s", endpoint, resp.Status)

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("reading response from %s: %w", endpoint, err)
	}
	if len(body) > 0 {
		if uerr := json.Unmarshal(body, out); uerr != nil {
			// A body that isn't valid JSON is only a hard error on success;
			// on a failure status the caller inspects the raw body (and status)
			// to build a useful message, so don't mask it with a decode error.
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp.StatusCode, body, fmt.Errorf("decoding response from %s: %w", endpoint, uerr)
			}
		}
	}
	return resp.StatusCode, body, nil
}

// oauthError extracts a human-readable message from an OAuth-style error
// response body (error / error_description fields), falling back to the raw
// body text when it is not the expected shape.
func oauthError(body []byte) string {
	var e struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	if len(body) > 0 && json.Unmarshal(body, &e) == nil && e.Error != "" {
		if e.ErrorDescription != "" {
			return fmt.Sprintf("%s: %s", e.Error, e.ErrorDescription)
		}
		return e.Error
	}
	return strings.TrimSpace(string(body))
}

// RequestDeviceCode calls the device authorization endpoint and returns the
// device/user codes and verification URIs.
func (c *Client) RequestDeviceCode(ctx context.Context) (*DeviceAuth, error) {
	endpoint, err := c.endpoint("auth/device")
	if err != nil {
		return nil, err
	}

	// PKCE (RFC 7636): send a code_challenge now and the matching
	// code_verifier when polling for the token. Required by Keycloak clients
	// with PKCE enforced.
	verifier, err := c.newVerifier()
	if err != nil {
		return nil, err
	}

	form := url.Values{}
	form.Set("client_id", c.ClientID)
	form.Set("code_challenge", challengeS256(verifier))
	form.Set("code_challenge_method", "S256")

	var da DeviceAuth
	status, body, err := c.postForm(ctx, endpoint, form, &da)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		if detail := oauthError(body); detail != "" {
			return nil, fmt.Errorf("device authorization request to %s failed: HTTP %d: %s",
				endpoint, status, detail)
		}
		return nil, fmt.Errorf("device authorization request to %s failed: HTTP %d (no response body)",
			endpoint, status)
	}
	if da.DeviceCode == "" {
		return nil, fmt.Errorf("device authorization response had no device_code")
	}
	// Keycloak may omit interval; default to 5s per RFC 8628.
	if da.Interval <= 0 {
		da.Interval = 5
	}
	da.codeVerifier = verifier
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
		if da.codeVerifier != "" {
			form.Set("code_verifier", da.codeVerifier)
		}

		var tr tokenResponse
		status, _, err := c.postForm(ctx, endpoint, form, &tr)
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
