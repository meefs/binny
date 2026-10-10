// Package ghidra runs the openbin ghidra-worker Docker image locally so BINNY
// can create and decompile projects without the backend (and without any AWS
// cost). It is a port of the openbin CLI's local-worker orchestration
// (cli/ghidra.go) with two deliberate differences:
//
//   - Output goes through a Logf callback, never straight to stderr — BINNY
//     is a fullscreen TUI and raw stderr writes would corrupt the display.
//   - The multipart request carries `processor` and `loader` fields so a
//     forced Ghidra language ID (firmware whose arch Ghidra can't autodetect)
//     reaches analyzeHeadless. Requires ghidra-worker:7+.
//
// The image tarball cache is SHARED with the openbin CLI
// ($XDG_DATA_HOME/openbin/ghidra-worker.tar.gz), so a user who has already
// decompiled with the CLI never re-downloads the ~700MB image for BINNY.
package ghidra

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DefaultImage is the worker tag BINNY runs. The tag is the cache key — keep it
// in lockstep with the openbin CLI's ghidraWorkerImage (cli/config.go) and the
// --tag in .github/workflows/release-cli.yml. :7 adds -processor/-loader
// passthrough for firmware whose arch Ghidra can't autodetect.
const DefaultImage = "openbin/ghidra-worker:7"

// ImageTarball is the release asset / cache filename (version-less on purpose;
// the tag inside is the version).
const ImageTarball = "ghidra-worker.tar.gz"

// Image resolves the worker image: OPENBIN_GHIDRA_IMAGE env wins, else default.
func Image() string {
	if v := os.Getenv("OPENBIN_GHIDRA_IMAGE"); v != "" {
		return v
	}
	return DefaultImage
}

// Logf receives human-readable progress lines (no trailing newline).
type Logf func(format string, args ...any)

// Request is one local decompile job.
type Request struct {
	BinaryPath string
	Arch       string // display label, round-tripped into metadata ("" → auto)
	Processor  string // Ghidra language ID to FORCE (e.g. ARM:LE:32:v7); "" → autodetect
	Loader     string // "" (autodetect) or "binary" (headerless/raw firmware; needs Processor)
	Image      string // "" → Image()

	// Optional wall-clock overrides in seconds (0 = worker defaults:
	// 1500s whole-run / 1200s auto-analysis).
	AnalyzeTimeoutSec int
	PerFileTimeoutSec int
}

// Analyze runs the full local pipeline: ensure the image, start a container on
// a loopback port, POST the binary, tear down. Returns the raw worker JSON —
// the exact bytes the cloud pipeline used to produce, ready to store locally
// or forward to ingest. ctx cancellation (Esc in the TUI) aborts the run.
func Analyze(ctx context.Context, req Request, logf Logf) ([]byte, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	image := req.Image
	if image == "" {
		image = Image()
	}
	if req.Loader == "binary" && req.Processor == "" {
		return nil, fmt.Errorf("loader=binary (raw image) requires a processor ID — Ghidra has no headers to autodetect from")
	}

	if err := EnsureImage(image, logf); err != nil {
		return nil, err
	}
	port, err := findFreePort()
	if err != nil {
		return nil, fmt.Errorf("allocate port: %w", err)
	}
	containerID, err := startContainer(image, port, req.env()...)
	if err != nil {
		return nil, err
	}
	defer stopContainer(containerID)

	if err := waitForHealth(ctx, port, 60*time.Second); err != nil {
		logTail(containerID, logf)
		return nil, err
	}

	logf("Ghidra analyzing %s… (this can take minutes; Esc cancels)", filepath.Base(req.BinaryPath))
	body, err := postBinary(ctx, port, req)
	if err != nil {
		logTail(containerID, logf)
		return nil, err
	}
	return body, nil
}

// env renders worker environment overrides for docker run.
func (r Request) env() []string {
	var e []string
	if r.AnalyzeTimeoutSec > 0 {
		e = append(e, fmt.Sprintf("GHIDRA_ANALYZE_TIMEOUT=%d", r.AnalyzeTimeoutSec))
	}
	if r.PerFileTimeoutSec > 0 {
		e = append(e, fmt.Sprintf("GHIDRA_PER_FILE_TIMEOUT=%d", r.PerFileTimeoutSec))
	}
	return e
}

// EnsureImage makes sure the Docker daemon has the worker image, mirroring the
// CLI's resolution order: loaded already → bundled/cached tarball → download
// from the latest GitHub release into the shared openbin cache dir.
func EnsureImage(image string, logf Logf) error {
	if err := dockerPreflight(); err != nil {
		return err
	}
	if dockerImageExists(image) {
		return nil
	}
	dest := filepath.Join(xdgDataHome(), "openbin", ImageTarball)
	if tarball, ok := findBundledImageTarball(); ok {
		logf("Loading worker image from %s (first run only)…", tarball)
		if err := loadImageTarball(image, tarball); err == nil {
			return nil
		}
		// Stale cache after a worker tag bump — the asset name is version-less,
		// so an old cached tarball loads the old tag. Re-download the current one.
		logf("Cached %s doesn't contain %s (stale after a worker update) — re-downloading…", ImageTarball, image)
	}
	if err := downloadWorkerImage(dest, logf); err != nil {
		return fmt.Errorf("worker image %q not available locally and download failed: %w\n"+
			"You can also drop %s in %s to install it offline.",
			image, err, ImageTarball, filepath.Dir(dest))
	}
	logf("Loading worker image (first run only)…")
	return loadImageTarball(image, dest)
}

func loadImageTarball(image, tarball string) error {
	if err := exec.Command("docker", "load", "-i", tarball).Run(); err != nil {
		return fmt.Errorf("docker load %s: %w", tarball, err)
	}
	if !dockerImageExists(image) {
		return fmt.Errorf("loaded %s but image %q still not found", tarball, image)
	}
	return nil
}

// dockerPreflight verifies Docker is installed AND the daemon answers, with
// platform-tailored guidance (same failure split as the CLI).
func dockerPreflight() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("%s", dockerMissingMessage())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info").CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("Docker is installed but its daemon isn't responding — start it and retry.\nDocker reported: %s", abbrev(detail, 300))
	}
	return nil
}

func dockerMissingMessage() string {
	switch runtime.GOOS {
	case "darwin", "windows":
		return "Docker is required for local decompiles but wasn't found on PATH. Install Docker Desktop (https://www.docker.com/products/docker-desktop/) and retry."
	default:
		return "Docker is required for local decompiles but wasn't found on PATH. Install Docker Engine (https://docs.docker.com/engine/install/) and add your user to the docker group."
	}
}

func dockerImageExists(image string) bool {
	return exec.Command("docker", "image", "inspect", image).Run() == nil
}

// findBundledImageTarball searches the shared openbin install locations.
func findBundledImageTarball() (string, bool) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		resolved := exe
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			resolved = real
		}
		candidates = append(candidates, filepath.Join(filepath.Dir(resolved), ImageTarball))
	}
	candidates = append(candidates,
		filepath.Join(xdgDataHome(), "openbin", ImageTarball),
		filepath.Join("/usr/local/share/openbin", ImageTarball),
	)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, ImageTarball))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, true
		}
	}
	return "", false
}

func xdgDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share")
	}
	return ""
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// startContainer launches the worker detached on a loopback port. No --rm so
// logs survive a crash for logTail; stopContainer removes it.
func startContainer(image string, port int, envVars ...string) (string, error) {
	runArgs := []string{"run", "-d", "-p", fmt.Sprintf("127.0.0.1:%d:8000", port)}
	for _, e := range envVars {
		runArgs = append(runArgs, "-e", e)
	}
	runArgs = append(runArgs, image)
	out, err := exec.Command("docker", runArgs...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("docker run failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("docker run: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func stopContainer(id string) {
	if id == "" {
		return
	}
	_ = exec.Command("docker", "stop", id).Run()
	_ = exec.Command("docker", "rm", "-f", id).Run()
}

// logTail surfaces the last worker log lines through logf on failure — the
// TUI-safe stand-in for the CLI's full post-mortem dump.
func logTail(id string, logf Logf) {
	if id == "" {
		return
	}
	out, _ := exec.Command("docker", "logs", "--tail", "25", id).CombinedOutput()
	if s := strings.TrimSpace(string(out)); s != "" {
		logf("ghidra-worker log tail:\n%s", s)
	}
}

func waitForHealth(ctx context.Context, port int, timeout time.Duration) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ghidra-worker did not become healthy within %s", timeout)
}

func postBinary(ctx context.Context, port int, req Request) ([]byte, error) {
	f, err := os.Open(req.BinaryPath)
	if err != nil {
		return nil, fmt.Errorf("open binary: %w", err)
	}
	defer f.Close()

	arch := req.Arch
	if arch == "" {
		arch = "auto"
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("arch", arch)
	if req.Processor != "" {
		_ = mw.WriteField("processor", req.Processor)
	}
	if req.Loader != "" {
		_ = mw.WriteField("loader", req.Loader)
	}
	part, err := mw.CreateFormFile("binary", filepath.Base(req.BinaryPath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, f); err != nil {
		return nil, fmt.Errorf("read binary: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	httpReq, _ := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://127.0.0.1:%d/analyze", port), &buf)
	httpReq.Header.Set("Content-Type", mw.FormDataContentType())
	httpReq.Header.Set("Accept", "application/json")

	// Worker's own wall is 25m by default; give it headroom, scaled with an
	// explicit override like the CLI does.
	httpTimeout := 30 * time.Minute
	if req.AnalyzeTimeoutSec > 0 {
		if d := time.Duration(req.AnalyzeTimeoutSec)*time.Second + 5*time.Minute; d > httpTimeout {
			httpTimeout = d
		}
	}
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("worker /analyze: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read worker response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("worker /analyze returned %d: %s", resp.StatusCode, abbrev(string(body), 700))
	}
	return body, nil
}

func abbrev(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
