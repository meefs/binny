package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"openbin.ai/binny/internal/api"
	"openbin.ai/binny/internal/app"
	"openbin.ai/binny/internal/tools"
)

// registerSampleTools adds list_samples + switch_sample for multi-sample BIN
// projects (a dropper + its payloads, firmware revisions — one project, N
// decompile results). The project's own binary is the "primary"; extras are
// read-only side analyses. switch_sample hot-swaps the toolbox blob, exactly
// like decompile_sample does, so every read tool serves the chosen sample.
// Not registered for local projects (no backend, no samples).
func registerSampleTools(reg *tools.Registry, sess *app.Session, tb *tools.Toolbox) {
	if sess.Project == nil || sess.Project.IsLocal() || sess.Project.Kind != api.KindBIN {
		return
	}
	reg.Add(&tools.Tool{
		Def: tools.Def("list_samples",
			"List the EXTRA samples attached to this project (a multi-sample project holds one primary binary plus N more, e.g. a dropper and its payloads, or firmware revisions). Shows which one the read tools currently serve. Call this when the analyst mentions another file/payload/stage, or to check whether the project holds more than one binary.",
			tools.Obj(map[string]any{})),
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			samples, err := sess.API.ListSamples(sess.Project.ID)
			if err != nil {
				return "", fmt.Errorf("list samples: %w", err)
			}
			return FormatSamples(sess.Project, samples, tb.ActiveSample()), nil
		},
	})
	reg.Add(&tools.Tool{
		Def: tools.Def("switch_sample",
			"Point ALL read tools (get_metadata, list_functions, get_function, get_strings, xrefs, …) at a different sample of this project. `sample` is a number from list_samples, a label/filename, an id, or 'primary' for the project's own binary. Findings and IOCs you record are still attached to the project — name which sample each came from in the evidence note.",
			tools.Obj(map[string]any{
				"sample": tools.StrProp("Which sample: '1', '2', … (list_samples order), a label or filename, a sample id, or 'primary'"),
			}, "sample")),
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var a struct{ Sample string }
			if err := json.Unmarshal(input, &a); err != nil {
				return "", err
			}
			samples, err := sess.API.ListSamples(sess.Project.ID)
			if err != nil {
				return "", fmt.Errorf("list samples: %w", err)
			}
			target, err := tools.ResolveSample(samples, a.Sample)
			if err != nil {
				return "", err
			}
			an, err := tb.SwitchSample(target)
			if err != nil {
				return "", err
			}
			sess.Sample = target
			return SwitchSummary(sess.Project, target, an), nil
		},
	})
}

// FormatSamples renders the sample list the way both the tool and the
// /samples command show it: primary first, then 1-based extras.
func FormatSamples(p *api.Project, samples []api.Sample, active *api.Sample) string {
	var b strings.Builder
	mark := func(on bool) string {
		if on {
			return "▸ "
		}
		return "  "
	}
	fmt.Fprintf(&b, "%sprimary  %s", mark(active == nil), p.Name)
	if p.Arch != "" || p.ExecutableFormat != "" {
		fmt.Fprintf(&b, "  [%s %s]", orUnknown(p.ExecutableFormat), orUnknown(p.Arch))
	}
	b.WriteString("\n")
	if len(samples) == 0 {
		b.WriteString("(no extra samples — this project holds a single binary)\n")
		return b.String()
	}
	for i, s := range samples {
		on := active != nil && active.ID == s.ID
		fmt.Fprintf(&b, "%s#%-6d  %s", mark(on), i+1, s.DisplayName())
		if s.Label != "" && s.OriginalFilename != "" && !strings.EqualFold(s.Label, s.OriginalFilename) {
			fmt.Fprintf(&b, " (%s)", s.OriginalFilename)
		}
		if s.Ready() {
			fmt.Fprintf(&b, "  [%s %s]", orUnknown(s.ExecutableFormat), orUnknown(s.Arch))
		} else {
			fmt.Fprintf(&b, "  [%s]", s.Status)
		}
		if s.Sha256 != "" {
			fmt.Fprintf(&b, "  sha256 %s…", s.Sha256[:minInt(12, len(s.Sha256))])
		}
		b.WriteString("\n")
	}
	b.WriteString("▸ = what the read tools serve now. switch_sample <#|label|primary> to change.\n")
	return b.String()
}

// SwitchSummary is the one-line confirmation after a switch.
func SwitchSummary(p *api.Project, s *api.Sample, a *api.Analysis) string {
	name, who := p.Name, "the PRIMARY binary"
	if s != nil {
		name, who = s.DisplayName(), "extra sample"
	}
	out := fmt.Sprintf("Now serving %s %q", who, name)
	if a != nil {
		out += fmt.Sprintf(": language=%s format=%s — %d functions, %d strings, %d imports",
			orUnknown(a.Metadata.Language), orUnknown(a.Metadata.ExecutableFormat),
			a.Metadata.FunctionCount, a.Metadata.StringCount, a.Metadata.ImportCount)
	}
	return out + ". All read tools now reflect this sample."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
