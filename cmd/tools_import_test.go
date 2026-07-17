package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestToolsImportIsGroup(t *testing.T) {
	out, err := execute(t, "tools", "import")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) == "UNIMPLEMENTED" {
			t.Errorf("`tools import` executed a stub; expected help:\n%s", out)
		}
	}
	for _, sub := range []string{"from-image", "from-source"} {
		if !strings.Contains(out, sub) {
			t.Errorf("`tools import` help missing subcommand %q:\n%s", sub, out)
		}
	}
}

func TestToolsImportFromSourceUnimplemented(t *testing.T) {
	out, err := execute(t, "tools", "import", "from-source")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "UNIMPLEMENTED") {
		t.Errorf("output = %q, want UNIMPLEMENTED", out)
	}
}

// newToolsImportServer serves /namespaces (for context validation) and
// captures the POST /tools body.
func newToolsImportServer(t *testing.T, gotBody *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/namespaces":
			_, _ = w.Write([]byte(`{"namespaces":["team1","team2"]}`))
		case r.URL.Path == "/api/v1/tools" && r.Method == http.MethodPost:
			_ = json.NewDecoder(r.Body).Decode(gotBody)
			_, _ = w.Write([]byte(`{"success":true,"name":"weather-mcp","namespace":"team1","message":"Tool created"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupToolsImportContext(t *testing.T, srv *httptest.Server, namespace string) {
	t.Helper()
	if _, err := execute(t, "config", "create-context",
		"--name", "dev", "--server", srv.URL+"/api/v1/"); err != nil {
		t.Fatalf("create-context: %v", err)
	}
	if _, err := execute(t, "config", "set-context", "--namespace", namespace); err != nil {
		t.Fatalf("set-context: %v", err)
	}
}

func TestToolsImportFromImagePostsRequest(t *testing.T) {
	isolateHome(t)
	var body map[string]any
	srv := newToolsImportServer(t, &body)
	setupToolsImportContext(t, srv, "team1")

	out, err := execute(t, "tools", "import", "from-image",
		"--name", "weather-mcp",
		"--containerImage", "ghcr.io/x/y:latest",
		"--imagePullSecret", "regcred",
	)
	if err != nil {
		t.Fatalf("import from-image: %v", err)
	}

	if body["name"] != "weather-mcp" || body["namespace"] != "team1" {
		t.Errorf("name/namespace wrong: %+v", body)
	}
	if body["deploymentMethod"] != "image" {
		t.Errorf("deploymentMethod = %v, want image", body["deploymentMethod"])
	}
	if body["workloadType"] != "deployment" {
		t.Errorf("workloadType = %v, want deployment (default)", body["workloadType"])
	}
	if body["containerImage"] != "ghcr.io/x/y:latest" || body["imagePullSecret"] != "regcred" {
		t.Errorf("image fields wrong: %+v", body)
	}
	if !strings.Contains(out, "Tool created") {
		t.Errorf("output missing server message:\n%s", out)
	}
}

func TestToolsImportFromImageDeploymentType(t *testing.T) {
	isolateHome(t)
	var body map[string]any
	srv := newToolsImportServer(t, &body)
	setupToolsImportContext(t, srv, "team1")

	if _, err := execute(t, "tools", "import", "--deployment-type", "statefulset", "from-image",
		"--name", "weather-mcp", "--containerImage", "img"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if body["workloadType"] != "statefulset" {
		t.Errorf("workloadType = %v, want statefulset", body["workloadType"])
	}
}

func TestToolsImportFromImageNamespaceOverride(t *testing.T) {
	isolateHome(t)
	var body map[string]any
	srv := newToolsImportServer(t, &body)
	setupToolsImportContext(t, srv, "team1")

	if _, err := execute(t, "tools", "--namespace", "team2", "import", "from-image",
		"--name", "weather-mcp", "--containerImage", "img"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if body["namespace"] != "team2" {
		t.Errorf("namespace = %v, want team2 (override)", body["namespace"])
	}
}

func TestToolsImportFromImageEnvVars(t *testing.T) {
	isolateHome(t)
	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/env":
			_, _ = w.Write([]byte("FOO=bar\n# comment\nBAZ=qux\n"))
		case r.URL.Path == "/api/v1/namespaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"namespaces":["team1"]}`))
		case r.URL.Path == "/api/v1/tools" && r.Method == http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{"success":true,"message":"ok"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	setupToolsImportContext(t, srv, "team1")

	if _, err := execute(t, "tools", "import", "from-image",
		"--name", "weather-mcp", "--containerImage", "img",
		"--envVarsURL", srv.URL+"/env"); err != nil {
		t.Fatalf("import: %v", err)
	}

	envVars, ok := body["envVars"].([]any)
	if !ok || len(envVars) != 2 {
		t.Fatalf("envVars = %+v, want 2 entries", body["envVars"])
	}
	first := envVars[0].(map[string]any)
	if first["name"] != "FOO" || first["value"] != "bar" {
		t.Errorf("envVars[0] = %+v, want {FOO bar}", first)
	}
}

func TestToolsImportFromImageRequiresNameAndImage(t *testing.T) {
	isolateHome(t)
	var body map[string]any
	srv := newToolsImportServer(t, &body)
	setupToolsImportContext(t, srv, "team1")

	if _, err := execute(t, "tools", "import", "from-image", "--containerImage", "img"); err == nil {
		t.Error("expected error when --name is missing")
	}
	if _, err := execute(t, "tools", "import", "from-image", "--name", "weather-mcp"); err == nil {
		t.Error("expected error when --containerImage is missing")
	}
}

func TestToolsImportDeploymentTypeDefault(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"tools", "import", "from-image"})
	if err != nil {
		t.Fatalf("could not find command: %v", err)
	}
	f := cmd.Flags().Lookup("deployment-type")
	if f == nil {
		t.Fatal("from-image does not inherit --deployment-type")
	}
	if f.DefValue != "deployment" {
		t.Errorf("--deployment-type default = %q, want deployment", f.DefValue)
	}
}

func TestToolsImportFromSourceGitBranchDefault(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"tools", "import", "from-source"})
	if err != nil {
		t.Fatalf("could not find command: %v", err)
	}
	f := cmd.Flags().Lookup("gitBranch")
	if f == nil {
		t.Fatal("from-source has no --gitBranch flag")
	}
	if f.DefValue != "main" {
		t.Errorf("--gitBranch default = %q, want %q", f.DefValue, "main")
	}
}
