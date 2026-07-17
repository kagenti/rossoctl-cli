package deviceflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newClient returns a Client pointed at srv with a no-op Sleep so polling
// tests run instantly.
func newClient(srv *httptest.Server) *Client {
	return &Client{
		KeycloakURL: srv.URL,
		Realm:       "rossoctl",
		ClientID:    "rossoctl-ui",
		Sleep:       func(time.Duration) {},
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

func TestEndpointErrors(t *testing.T) {
	if _, err := (&Client{Realm: "r"}).RequestDeviceCode(context.Background()); err == nil {
		t.Error("expected error with empty KeycloakURL")
	}
	if _, err := (&Client{KeycloakURL: "http://k"}).RequestDeviceCode(context.Background()); err == nil {
		t.Error("expected error with empty Realm")
	}
}
