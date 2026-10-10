package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"openbin.ai/binny/internal/app"
	"openbin.ai/binny/internal/sandbox"
	"openbin.ai/binny/internal/store"
	"openbin.ai/binny/internal/tools"
)

// registerSandboxTools adds the active-extraction toolset: write_file, run, and
// read_artifact. These operate inside a hardened disposable container (STATIC
// tier — the sample is read as data, never executed). This is the headline
// paid capability: BINNY writes an extraction script, runs it against the real
// sample, iterates on failure, validates, and folds the decrypted config + IOCs
// into the report.
func registerSandboxTools(reg *tools.Registry, inv *Investigation, sess *app.Session) *sandboxHolder {
	sb := &sandboxHolder{sess: sess}

	reg.Add(&tools.Tool{
		Def: tools.Def("write_file",
			"Write a script or helper into the extraction workspace (/work — a persistent host directory, so artifacts survive across sessions). Use for a Python/shell extraction script you'll then run against the sample. `path` is relative to /work.",
			tools.Obj(map[string]any{
				"path":    tools.StrProp("workspace-relative path, e.g. extract.py"),
				"content": tools.StrProp("file contents"),
			}, "path", "content")),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var a struct{ Path, Content string }
			if err := json.Unmarshal(input, &a); err != nil {
				return "", err
			}
			s, err := sb.get()
			if err != nil {
				return "", err
			}
			if err := s.WriteFile(a.Path, a.Content); err != nil {
				return "", err
			}
			inv.AddArtifact(Artifact{Path: a.Path, Kind: "script", Description: "extraction script", Bytes: []byte(a.Content)})
			return "wrote " + a.Path, nil
		},
	})

	reg.Add(&tools.Tool{
		Def: tools.Def("run",
			"Run a shell command inside the hardened sandbox (STATIC tier — no network, non-root, read-only sample at /sample, persistent workspace at /work). Available: readelf, objdump, nm, strings, xxd, file, python3, radare2, binwalk (+ squashfs/7z/xz/zstd carving tools for firmware). The sample is DATA to read, not a program to execute. Default timeout 120s; pass timeout_sec (max 900) for long jobs like `binwalk -e` on a big firmware image. Returns stdout, stderr, exit code.",
			tools.Obj(map[string]any{
				"cmd": tools.StrProp("shell command, e.g. python3 extract.py, or binwalk /sample"),
				"timeout_sec": map[string]any{"type": "integer",
					"description": "optional timeout in seconds (default 120, max 900) for long-running extraction/carving"},
			}, "cmd")),
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var a struct {
				Cmd        string `json:"cmd"`
				TimeoutSec int    `json:"timeout_sec"`
			}
			if err := json.Unmarshal(input, &a); err != nil {
				return "", err
			}
			s, err := sb.get()
			if err != nil {
				return "", err
			}
			res, err := s.Run(ctx, a.Cmd, time.Duration(a.TimeoutSec)*time.Second)
			if err != nil {
				return "", err
			}
			return formatRun(res), nil
		},
	})

	reg.Add(&tools.Tool{
		Def: tools.Def("read_artifact",
			"Read a file produced in the workspace (e.g. a decrypted config the script wrote). Returns its text (binary is shown as a hex/printable preview). Attaches it to the report draft as a decrypted artifact.",
			tools.Obj(map[string]any{
				"path":        tools.StrProp("workspace-relative path of the produced file"),
				"description": tools.StrProp("what this artifact is (e.g. 'decrypted DSLogdRAT config')"),
			}, "path")),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var a struct{ Path, Description string }
			if err := json.Unmarshal(input, &a); err != nil {
				return "", err
			}
			s, err := sb.get()
			if err != nil {
				return "", err
			}
			data, err := s.ReadArtifact(a.Path)
			if err != nil {
				return "", err
			}
			desc := a.Description
			if desc == "" {
				desc = "extracted artifact"
			}
			inv.AddArtifact(Artifact{Path: a.Path, Kind: "decrypted", Description: desc, Bytes: data})
			return previewBytes(data), nil
		},
	})

	return sb
}

// ResolveWorkspace returns the effective workspace dir for a session: the
// explicit /workspace choice, else the persistent per-sample default under
// ~/.local/share/binny/workspaces/ (falling back to name "default" when no
// sample is attached yet).
func ResolveWorkspace(sess *app.Session) (string, error) {
	if w := strings.TrimSpace(sess.WorkspacePath); w != "" {
		return w, nil
	}
	name := "default"
	if sp := strings.TrimSpace(sess.SamplePath); sp != "" {
		name = filepath.Base(sp)
	} else if sess.Project != nil {
		name = sess.Project.Name
	}
	return store.WorkspaceFor(name)
}

// sandboxHolder lazily creates the sandbox on first extraction tool use (so a
// session that never extracts pays nothing), and ensures the image exists. It
// reads the sample path AND workspace from the session at use time, so a
// mid-run /sample or /workspace takes effect on the next extraction: when
// either differs from what the cached sandbox was built with, it is rebuilt.
type sandboxHolder struct {
	sess *app.Session
	mu   sync.Mutex
	sb   *sandbox.Sandbox

	// what the cached sandbox was built with
	sample string
	work   string
}

func (h *sandboxHolder) get() (*sandbox.Sandbox, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	path := strings.TrimSpace(h.sess.SamplePath)
	if path == "" {
		return nil, fmt.Errorf("no local sample attached — extraction runs a script against the raw binary, which BINNY doesn't have. Ask the analyst to attach it with `/sample <path>` (the file they ran `openbin` on), then retry")
	}
	work, err := ResolveWorkspace(h.sess)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	if h.sb != nil && h.sample == path && h.work == work {
		return h.sb, nil
	}
	if h.sb != nil {
		_ = h.sb.Close() // no-op for persistent workspaces
	}
	if err := sandbox.EnsureImage(context.Background()); err != nil {
		return nil, err
	}
	sb, err := sandbox.New(path, work)
	if err != nil {
		return nil, err
	}
	h.sb, h.sample, h.work = sb, path, work
	return h.sb, nil
}

func formatRun(r *sandbox.RunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "exit=%d", r.ExitCode)
	if r.TimedOut {
		b.WriteString(" (TIMED OUT — pass a larger timeout_sec if the job legitimately needs longer)")
	}
	b.WriteByte('\n')
	if s := strings.TrimRight(r.Stdout, "\n"); s != "" {
		b.WriteString("stdout:\n")
		b.WriteString(truncateStr(s, 8000))
		b.WriteByte('\n')
	}
	if s := strings.TrimRight(r.Stderr, "\n"); s != "" {
		b.WriteString("stderr:\n")
		b.WriteString(truncateStr(s, 2000))
		b.WriteByte('\n')
	}
	return b.String()
}

func previewBytes(data []byte) string {
	const max = 4000
	printable := true
	for _, c := range data {
		if c != '\t' && c != '\n' && c != '\r' && (c < 0x20 || c > 0x7e) {
			printable = false
			break
		}
	}
	if printable {
		return fmt.Sprintf("(%d bytes, text)\n%s", len(data), truncateStr(string(data), max))
	}
	// hexdump-ish preview
	n := len(data)
	if n > 512 {
		n = 512
	}
	var b strings.Builder
	fmt.Fprintf(&b, "(%d bytes, binary — first %d as hex)\n", len(data), n)
	for i := 0; i < n; i += 16 {
		end := i + 16
		if end > n {
			end = n
		}
		for j := i; j < end; j++ {
			fmt.Fprintf(&b, "%02x ", data[j])
		}
		b.WriteString("  ")
		for j := i; j < end; j++ {
			c := data[j]
			if c >= 0x20 && c <= 0x7e {
				b.WriteByte(c)
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}
