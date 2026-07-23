package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/rossoctl/rossoctl-cli/internal/cortexclient"
)

// credentialNames is the ordered set of environment variables / credential
// files rossocortex accepts for a LiteLLM/LLM key (mirrors rossoctlx's
// CREDENTIAL_NAMES).
var credentialNames = []string{
	"LITELLM_API_KEY",
	"ROSSOCORTEX_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"OPENAI_API_KEY",
}

// doctorArgs holds the `cortex doctor` flags. They mirror the subset of start
// flags cmd_doctor reads (args.image, args.local).
var doctorArgs struct {
	image string
	local bool
}

// doctorCheck is one preflight result. A nil ok means a non-fatal warning.
type doctorCheck struct {
	ok     *bool
	name   string
	detail string
	fix    string
}

func boolPtr(b bool) *bool { return &b }

var cortexDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the environment for running a cortex",
	Long: `Offline environment preflight for running a cortex — pass/fail with
remediation hints. Exits non-zero when any check fails.

Checks git and a container runtime on PATH, the runtime daemon's health, the
default image's architecture, an available LiteLLM/LLM credential, the upstream
URL, config-dir writability, and whether the default ports are free. With
--local it also checks the native-mode prerequisites.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runDoctor(cmd)
	},
}

// runDoctor performs the environment preflight, prints the results, and returns
// an error (so the process exits non-zero) when any check fails.
func runDoctor(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "rossoctl cortex doctor — environment preflight")
	fmt.Fprintln(out)

	fc := cortexclient.NewFileClient(cortexName)
	var results []doctorCheck
	add := func(ok *bool, name, detail, fix string) {
		results = append(results, doctorCheck{ok, name, detail, fix})
	}

	// git — install-time dependency.
	git := lookPath("git")
	add(warnUnless(git != ""), "git on PATH", orNotFound(git),
		"git is needed to (re)install from the Git repo — install git")

	// // rossoctl itself resolvable on PATH.
	// rx := lookPath("rossoctl")
	// add(warnUnless(rx != ""), "rossoctl on PATH", orElse(rx, "not found (running via script?)"),
	//	"if installed via the download script, add its directory to your PATH")

	// container runtime + daemon health.
	rt := findContainerRuntime()
	if rt != "" {
		up := daemonOK(rt)
		add(boolPtr(up), fmt.Sprintf("container runtime (%s)", rt),
			ternary(up, "daemon responding", "installed but daemon NOT responding"),
			"start Docker Desktop, or `sudo systemctl start docker` / `systemctl --user start podman`")
	} else {
		// A missing runtime is fatal unless --local was requested.
		add(warnIf(doctorArgs.local), "container runtime (docker/podman)", "none on PATH",
			"install docker or podman for container mode — or use --local")
	}

	// architecture vs image.
	host := normArch(runtime.GOARCH)
	img := doctorArgs.image
	imgArch := ""
	if rt != "" {
		if a, ok := inspectImageArch(rt, img); ok {
			imgArch = normArch(a)
		}
	}
	switch {
	case imgArch != "":
		add(boolPtr(imgArch == host), fmt.Sprintf("image arch matches host (%s)", host),
			fmt.Sprintf("%s is %s", img, imgArch),
			"build a native image and pass --image")
	case strings.HasPrefix(img, "quay.io/aslomnet/rosscortex") && host != "arm64":
		add(nil, fmt.Sprintf("image arch vs host (%s)", host),
			"default image is linux/arm64-only, not yet pulled",
			fmt.Sprintf("on %s build a native image and pass --image", host))
	}

	// credential.
	credsDir := filepath.Join(fc.ConfigDir, "credentials")
	cred := findCredential(credsDir)
	add(boolPtr(cred != ""), "LiteLLM/LLM API key", orElse(cred, "none found"),
		fmt.Sprintf("set one of %s (file in %s or env)",
			strings.Join(credentialNames, ", "), credsDir))

	// upstream.
	upstream := firstNonEmpty(
		os.Getenv("ROSSOCORTEX_UPSTREAM"),
		os.Getenv("ANTHROPIC_BASE_URL"),
		stateUpstream(fc.StateFilename),
	)
	add(warnUnless(upstream != ""), "upstream LLM URL", orElse(upstream, "not set"),
		"pass --upstream <URL> or export ROSSOCORTEX_UPSTREAM")

	// config dir writable.
	add(boolPtr(dirWritable(fc.ConfigDir)), "config dir writable", configDirNote(fc.ConfigDir),
		fmt.Sprintf("ensure %s is writable", fc.ConfigDir))

	// default ports.
	pFree, cFree := portIsFree(defaultProxyPort), portIsFree(defaultProxyPort+1)
	add(warnUnless(pFree && cFree), "default ports free",
		fmt.Sprintf("proxy %d %s, control %d %s",
			defaultProxyPort, freeInUse(pFree), defaultProxyPort+1, freeInUse(cFree)),
		"start auto-picks free ports if these are taken")

	// native-mode prerequisites (optional).
	if doctorArgs.local {
		uv := lookPath("uv")
		add(warnUnless(uv != ""), "uv on PATH (--local)", orNotFound(uv),
			"install uv: https://astral.sh/uv")
		localDir := os.Getenv("ROSSOCORTEX_CONTAINER_LOCAL_DIR")
		add(warnUnless(localDir != ""), "ROSSOCORTEX_CONTAINER_LOCAL_DIR (--local)", orElse(localDir, "not set"),
			"export ROSSOCORTEX_CONTAINER_LOCAL_DIR=<checkout>/scripts/rossocortex-container")
	} else {
		fmt.Fprintln(out, "  (native-mode checks skipped; pass --local to include them)")
		fmt.Fprintln(out)
	}

	return reportDoctor(out, results)
}

// reportDoctor prints each check with a mark, tallies the results, and returns
// an error when any check failed.
func reportDoctor(out io.Writer, results []doctorCheck) error {
	fails, warns, passes := 0, 0, 0
	for _, r := range results {
		mark := "✗"
		switch {
		case r.ok == nil:
			mark = "!"
		case *r.ok:
			mark = "✓"
		}
		line := fmt.Sprintf("  [%s] %s", mark, r.name)
		if r.detail != "" {
			line += fmt.Sprintf("  (%s)", r.detail)
		}
		fmt.Fprintln(out, line)
		switch {
		case r.ok == nil:
			warns++
			if r.fix != "" {
				fmt.Fprintf(out, "      note: %s\n", r.fix)
			}
		case *r.ok:
			passes++
		default:
			fails++
			if r.fix != "" {
				fmt.Fprintf(out, "      fix: %s\n", r.fix)
			}
		}
	}

	fmt.Fprintf(out, "\n%d passed, %d warnings, %d failed\n", passes, warns, fails)
	if fails == 0 {
		fmt.Fprintln(out, "Ready. Next: rossoctl cortex start --upstream <URL>")
		return nil
	}
	return fmt.Errorf("%d check(s) failed", fails)
}

// --- helpers (ports of rossoctlx's doctor helpers) ---

// warnUnless returns a *bool that is true when cond holds; when cond is false
// it returns nil, marking the check as a non-fatal warning rather than a
// failure. Used for checks the reference treats as warnings.
func warnUnless(cond bool) *bool {
	if cond {
		return boolPtr(true)
	}
	return nil
}

// warnIf returns nil (warning) when cond holds, otherwise a false pointer
// (failure). Used for the container-runtime check: a missing runtime is only a
// warning under --local.
func warnIf(cond bool) *bool {
	if cond {
		return nil
	}
	return boolPtr(false)
}

// lookPath returns the resolved path of an executable, or "" if not found.
func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// findContainerRuntime resolves the container runtime: $ROSSOCORTEX_RUNTIME if
// set and on PATH, else docker, else podman, else "".
func findContainerRuntime() string {
	if pref := os.Getenv("ROSSOCORTEX_RUNTIME"); pref != "" && lookPath(pref) != "" {
		return pref
	}
	for _, c := range []string{"docker", "podman"} {
		if lookPath(c) != "" {
			return c
		}
	}
	return ""
}

// daemonOK reports whether `<runtime> info` succeeds within a short timeout.
func daemonOK(rt string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, rt, "info").Run() == nil
}

// inspectImageArch returns the architecture of a local image, or ("", false)
// if it cannot be determined (image not pulled, runtime error, timeout).
func inspectImageArch(rt, image string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	outBytes, err := exec.CommandContext(ctx, rt, "image", "inspect", image, "--format", "{{.Architecture}}").Output()
	if err != nil {
		return "", false
	}
	arch := strings.TrimSpace(string(outBytes))
	if arch == "" {
		return "", false
	}
	return arch, true
}

// normArch normalizes an architecture string to "arm64" / "amd64", or returns
// it lowercased unchanged.
func normArch(m string) string {
	m = strings.ToLower(m)
	switch m {
	case "arm64", "aarch64":
		return "arm64"
	case "x86_64", "amd64":
		return "amd64"
	}
	return m
}

// dirWritable reports whether path can be created and written to, probing with
// a temp file it then removes.
func dirWritable(path string) bool {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return false
	}
	probe := filepath.Join(path, ".doctor-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// findCredential returns a description of the first available LiteLLM/LLM
// credential (a file in credsDir, then an env var), or "" if none is found.
func findCredential(credsDir string) string {
	for _, name := range credentialNames {
		f := filepath.Join(credsDir, name)
		if b, err := os.ReadFile(f); err == nil && strings.TrimSpace(string(b)) != "" {
			return "file " + f
		}
	}
	for _, name := range credentialNames {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			return "env $" + name
		}
	}
	return ""
}

// stateUpstream reads the "upstream" value from the rossocortex state file, or
// "" if the file is absent, unreadable, or lacks the key.
func stateUpstream(stateFile string) string {
	b, err := os.ReadFile(stateFile)
	if err != nil {
		return ""
	}
	var state struct {
		Upstream string `json:"upstream"`
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return ""
	}
	return state.Upstream
}

// portIsFree reports whether nothing is listening on 127.0.0.1:port.
func portIsFree(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return true
	}
	_ = conn.Close()
	return false
}

// configDirNote describes the active config dir and which override selected it.
func configDirNote(dir string) string {
	var src string
	switch {
	case os.Getenv("ROSSOCORTEX_CONFIG_DIR") != "":
		src = "ROSSOCORTEX_CONFIG_DIR override active"
	case os.Getenv("XDG_CONFIG_HOME") != "":
		src = "XDG_CONFIG_HOME override active"
	default:
		src = "default (~/.config)"
	}
	return fmt.Sprintf("Config dir: %s  [%s]", dir, src)
}

// --- small formatting helpers ---

func orNotFound(s string) string { return orElse(s, "not found") }

func orElse(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func freeInUse(free bool) string { return ternary(free, "free", "in use") }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func init() {
	cortexDoctorCmd.Flags().StringVar(&doctorArgs.image, "image", defaultCortexImage, "Container image to check")
	cortexDoctorCmd.Flags().BoolVar(&doctorArgs.local, "local", false, "Include native-mode (--local) prerequisite checks")
}
