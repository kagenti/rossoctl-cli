package deviceflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// testVerifier is a fixed PKCE code_verifier injected into test clients so the
// derived code_challenge is deterministic.
const testVerifier = "test-code-verifier-0123456789abcdef"

// newClient returns a Client pointed at srv with a no-op Sleep and a fixed
// PKCE verifier so polling tests run instantly and deterministically.
func newClient(srv *httptest.Server) *Client {
	return &Client{
		KeycloakURL: srv.URL,
		Realm:       "rossoctl",
		ClientID:    "rossoctl-ui",
		Sleep:       func(time.Duration) {},
		NewVerifier: func() (string, error) { return testVerifier, nil },
	}
}

func TestRequestDeviceCode(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"device_code": "DEV123",
			"user_code": "WDJB-MJHT",
			"verification_uri": "https://kc/realms/rossoctl/device",
			"verification_uri_complete": "https://kc/realms/rossoctl/device?user_code=WDJB-MJHT",
			"expires_in": 600,
			"interval": 5
		}`))
	}))
	defer srv.Close()

	da, err := newClient(srv).RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/realms/rossoctl/protocol/openid-connect/auth/device" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, "client_id=rossoctl-ui") {
		t.Errorf("body = %q, want client_id", gotBody)
	}
	if da.DeviceCode != "DEV123" || da.UserCode != "WDJB-MJHT" {
		t.Errorf("unexpected device auth: %+v", da)
	}
	if da.VerificationURIComplete == "" {
		t.Error("missing verification_uri_complete")
	}
}

func TestRequestDeviceCodeDefaultsInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"device_code":"D","user_code":"U","verification_uri":"http://v"}`))
	}))
	defer srv.Close()

	da, err := newClient(srv).RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if da.Interval != 5 {
		t.Errorf("interval = %d, want default 5", da.Interval)
	}
}

func TestPKCERoundTrip(t *testing.T) {
	var deviceForm, tokenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/device"):
			deviceForm = r.Form
			_, _ = w.Write([]byte(`{"device_code":"DEV","user_code":"U","verification_uri":"http://v","interval":1}`))
		case strings.HasSuffix(r.URL.Path, "/token"):
			tokenForm = r.Form
			_, _ = w.Write([]byte(`{"access_token":"T"}`))
		}
	}))
	defer srv.Close()

	c := newClient(srv)
	da, err := c.RequestDeviceCode(context.Background())
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}

	// The device request must carry the S256 challenge derived from the
	// verifier, and the method.
	if got := deviceForm.Get("code_challenge_method"); got != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", got)
	}
	wantChallenge := challengeS256(testVerifier)
	if got := deviceForm.Get("code_challenge"); got != wantChallenge {
		t.Errorf("code_challenge = %q, want %q", got, wantChallenge)
	}
	if deviceForm.Get("code_verifier") != "" {
		t.Error("code_verifier must not be sent in the device request")
	}

	if _, err := c.PollToken(context.Background(), da); err != nil {
		t.Fatalf("PollToken: %v", err)
	}
	// The token request must carry the matching verifier.
	if got := tokenForm.Get("code_verifier"); got != testVerifier {
		t.Errorf("token code_verifier = %q, want %q", got, testVerifier)
	}
}

func TestChallengeS256Known(t *testing.T) {
	// RFC 7636 Appendix B test vector.
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := challengeS256(verifier); got != want {
		t.Errorf("challengeS256 = %q, want %q (RFC 7636 vector)", got, want)
	}
}

func TestGenerateVerifierUnique(t *testing.T) {
	a, err := generateVerifier()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := generateVerifier()
	if a == b {
		t.Error("generateVerifier returned identical values")
	}
	if len(a) < 43 {
		t.Errorf("verifier length = %d, want >= 43 (RFC 7636 minimum)", len(a))
	}
}

func TestPollTokenPendingThenSuccess(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/token") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if g := r.Form.Get("grant_type"); g != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant_type = %q", g)
		}
		if r.Form.Get("device_code") != "DEV123" {
			t.Errorf("device_code = %q", r.Form.Get("device_code"))
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"THE-TOKEN"}`))
	}))
	defer srv.Close()

	tok, err := newClient(srv).PollToken(context.Background(), &DeviceAuth{DeviceCode: "DEV123", Interval: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "THE-TOKEN" {
		t.Errorf("token = %q, want THE-TOKEN", tok)
	}
	if calls != 3 {
		t.Errorf("polled %d times, want 3", calls)
	}
}

func TestPollTokenSlowDown(t *testing.T) {
	var sleeps []time.Duration
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"slow_down"}`))
		default:
			_, _ = w.Write([]byte(`{"access_token":"T"}`))
		}
	}))
	defer srv.Close()

	c := newClient(srv)
	c.Sleep = func(d time.Duration) { sleeps = append(sleeps, d) }

	tok, err := c.PollToken(context.Background(), &DeviceAuth{DeviceCode: "D", Interval: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok != "T" {
		t.Errorf("token = %q", tok)
	}
	// First sleep 5s, then bumped to 10s after slow_down.
	if len(sleeps) != 2 || sleeps[0] != 5*time.Second || sleeps[1] != 10*time.Second {
		t.Errorf("sleeps = %v, want [5s 10s]", sleeps)
	}
}

func TestPollTokenExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"expired_token"}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).PollToken(context.Background(), &DeviceAuth{DeviceCode: "D", Interval: 1})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("err = %v, want expired", err)
	}
}

func TestPollTokenAccessDenied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).PollToken(context.Background(), &DeviceAuth{DeviceCode: "D", Interval: 1})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Errorf("err = %v, want denied", err)
	}
}

func TestPollTokenContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := newClient(srv)
	// Cancel on the first sleep so polling stops promptly.
	c.Sleep = func(time.Duration) { cancel() }

	_, err := c.PollToken(ctx, &DeviceAuth{DeviceCode: "D", Interval: 1})
	if err == nil {
		t.Error("expected error after context cancel")
	}
}

func TestRequestDeviceCodeErrorIncludesDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client","error_description":"Invalid client or Invalid client credentials"}`))
	}))
	defer srv.Close()

	_, err := newClient(srv).RequestDeviceCode(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"400", "invalid_client", "Invalid client credentials", "/auth/device"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func TestRequestDeviceCodeErrorNonJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Bad Request"))
	}))
	defer srv.Close()

	_, err := newClient(srv).RequestDeviceCode(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	// The raw body text should be surfaced when it isn't OAuth-shaped JSON.
	if !strings.Contains(err.Error(), "Bad Request") {
		t.Errorf("error %q should include the raw body", err.Error())
	}
}

func TestEndpointErrors(t *testing.T) {
	if _, err := (&Client{Realm: "r"}).RequestDeviceCode(context.Background()); err == nil {
		t.Error("expected error with empty KeycloakURL")
	}
	if _, err := (&Client{KeycloakURL: "http://k"}).RequestDeviceCode(context.Background()); err == nil {
		t.Error("expected error with empty Realm")
	}
}
