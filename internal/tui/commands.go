package tui

import (
	"fmt"
	"sort"
	"strings"

	"openbin.ai/binny/internal/app"
	"openbin.ai/binny/internal/config"
)

// slashResult is what a slash command produces: transcript lines to append and
// whether the app should quit. Commands never do I/O directly here — anything
// async (like re-listing projects) returns a follow-up via the model.
type slashResult struct {
	lines []line
	quit  bool
	// openPicker re-opens the project picker (for /project).
	openPicker bool
}

// handleSlash dispatches a "/..." input line against the session, mutating
// state and returning transcript output. Unknown commands are reported, not
// fatal.
func (m *Model) handleSlash(input string) slashResult {
	fields := strings.Fields(input)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]
	s := m.sess

	switch cmd {
	case "/help", "/?":
		return slashResult{lines: helpLines()}

	case "/quit", "/exit", "/q":
		return slashResult{lines: []line{sysLine("Bye.")}, quit: true}

	case "/mode":
		if len(args) == 0 {
			return slashResult{lines: []line{sysLine(fmt.Sprintf(
				"Mode: %s. Usage: /mode malware | vuln", modeLabel(s.Mode)))}}
		}
		switch strings.ToLower(args[0]) {
		case "malware", "mar", "m":
			s.Mode = app.ModeMalware
		case "vuln", "vulnerability", "vrr", "vr", "v":
			s.Mode = app.ModeVuln
		default:
			return slashResult{lines: []line{errLine(fmt.Sprintf(
				"unknown mode %q — use 'malware' or 'vuln'", args[0]))}}
		}
		return slashResult{lines: []line{okLine("Mode → " + modeLabel(s.Mode))}}

	case "/models":
		return slashResult{lines: modelsLines(m)}

	case "/model":
		if len(args) == 0 {
			return slashResult{lines: []line{sysLine(fmt.Sprintf(
				"Current: %s / %s. Usage: /model <providerId | modelId>",
				providerID(m), s.Model))}}
		}
		return slashResult{lines: m.switchModel(args[0])}

	case "/endpoint", "/baseurl":
		return slashResult{lines: m.cmdEndpoint(args)}

	case "/project", "/open":
		return slashResult{openPicker: true}

	case "/intel":
		if len(args) == 0 {
			if s.Intel == "" {
				return slashResult{lines: []line{sysLine("No intel recorded. Usage: /intel <free text>")}}
			}
			return slashResult{lines: []line{sysLine("Intel: " + s.Intel)}}
		}
		s.Intel = strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
		return slashResult{lines: []line{okLine("Intel recorded.")}}

	case "/status", "/info":
		return slashResult{lines: statusLines(m)}

	default:
		return slashResult{lines: []line{errLine(fmt.Sprintf(
			"unknown command %q — /help for the list", cmd))}}
	}
}

func (m *Model) switchModel(arg string) []line {
	s := m.sess
	// Prefer interpreting the arg as a provider id (switches provider + default
	// model); fall back to a model id.
	if p := s.Config.Find(arg); p != nil {
		if err := s.SetProvider(arg); err != nil {
			return []line{errLine(err.Error())}
		}
	} else if err := s.SetModel(arg); err != nil {
		return []line{errLine(err.Error())}
	}
	return []line{okLine(fmt.Sprintf("Now using %s / %s", providerID(m), s.Model))}
}

// cmdEndpoint shows or changes the API root the active provider talks to.
//
//	/endpoint                 show the current and shipped endpoint
//	/endpoint <url>           point the active provider at <url>
//	/endpoint <provider> <url>
//	/endpoint reset           back to the shipped endpoint
//
// This is how a hosted gateway for a frontier model (your own
// anthropic.<host> that fronts the Messages API, an OpenAI-compatible
// router, a self-hosted server) gets used: same adapter, same key header,
// different host. The /models cache is keyed by base URL, so the model list
// refreshes on the next /model pick without a restart.
func (m *Model) cmdEndpoint(args []string) []line {
	s := m.sess
	target := s.Provider
	if len(args) == 2 {
		target = s.Config.Find(args[0])
		if target == nil {
			return []line{errLine(fmt.Sprintf("unknown provider %q — /models lists them", args[0]))}
		}
		args = args[1:]
	}
	if target == nil {
		return []line{errLine("no active provider — /model to pick one first")}
	}
	shipped := config.DefaultBaseURL(target.ID)
	if len(args) == 0 {
		out := []line{sysLine(fmt.Sprintf("%s endpoint: %s", target.ID, target.BaseURL))}
		if shipped != "" && shipped != target.BaseURL {
			out = append(out, sysLine("  shipped default: "+shipped+"  (/endpoint reset to go back)"))
		}
		out = append(out, sysLine("Usage: /endpoint <url> | /endpoint <provider> <url> | /endpoint reset"))
		return out
	}
	if strings.EqualFold(args[0], "reset") {
		if shipped == "" {
			return []line{errLine(fmt.Sprintf("%s has no shipped endpoint to reset to", target.ID))}
		}
		if err := s.Config.SetBaseURL(target.ID, shipped); err != nil {
			return []line{errLine(err.Error())}
		}
		return []line{okLine(fmt.Sprintf("%s endpoint → %s (shipped default)", target.ID, shipped))}
	}
	if err := s.Config.SetBaseURL(target.ID, args[0]); err != nil {
		return []line{errLine(err.Error())}
	}
	out := []line{okLine(fmt.Sprintf("%s endpoint → %s", target.ID, target.BaseURL))}
	if target.Kind == config.KindAnthropic {
		out = append(out, sysLine("Requests go to "+target.BaseURL+"/v1/messages with your key in x-api-key."))
	} else {
		out = append(out, sysLine("Requests go to "+target.BaseURL+"/chat/completions with your key as a Bearer token."))
	}
	return out
}

func modeLabel(mode app.Mode) string {
	if mode == app.ModeVuln {
		return "VULN_RESEARCH (VRR)"
	}
	return "MALWARE (MAR)"
}

func providerID(m *Model) string {
	if m.sess.Provider == nil {
		return "(none)"
	}
	return m.sess.Provider.ID
}

func helpLines() []line {
	return []line{
		sysLine("Commands:"),
		sysLine("  /investigate           start BINNY's analysis (triage → playbook → investigate → draft)"),
		sysLine("  /ask                   Q&A mode — question the sample on demand (verify a claim, build a PoC, inspect a function)"),
		sysLine("  /continue              resume from a checkpoint pause (Supervised/Collaborative)"),
		sysLine("  /autonomy <mode>       run mode: autonomous | supervised | collaborative"),
		sysLine("  /stop                  stop investigating early and synthesize the draft now"),
		sysLine("  /draft                 show the current draft report"),
		sysLine("  /findings              show recorded findings, IOCs, classification, coverage"),
		sysLine("  /save                  push the draft to the project report (DRAFT — review first)"),
		sysLine("  /sample [path]         attach the raw binary on disk to enable active extraction"),
		sysLine("  /decompile <path> [proc]  decompile a binary locally with Ghidra (kept on disk, no upload; proc = Ghidra ID like ARM:LE:32:v7 for raw firmware)"),
		sysLine("  /samples               list this project's extra samples (dropper + payloads, firmware revisions)"),
		sysLine("  /switch <#|label>      point BINNY's read tools at another sample ('primary' = the project's own binary)"),
		sysLine("  /workspace [path]      show/set the persistent sandbox workspace mounted at /work"),
		sysLine("  /refresh               drop this backend project's cached analysis blob"),
		sysLine("  /mode malware|vuln     switch analysis lens (MAR / VRR)"),
		sysLine("  /models                list configured LLM providers + models"),
		sysLine("  /model [id]            switch model/provider — no arg opens the ↑/↓ picker"),
		sysLine("  /key [provider]        set/update the BYOK API key (opens key entry)"),
		sysLine("  /endpoint [url|reset]  show/set the API root for the active provider (proxies, gateways, self-host)"),
		sysLine("  /project               re-open the project picker"),
		sysLine("  /settings              open Settings (LLM · keys · token budget)"),
		sysLine("  /budget [n]            show/set this project's token budget (e.g. /budget 200k)"),
		sysLine("  /intel [text]          record analyst intel for this sample"),
		sysLine("  /status                show current session state"),
		sysLine("  /help                  this help"),
		sysLine("  /quit                  exit BINNY"),
		sysLine("Type a message to start BINNY (your text becomes opening intel). While it runs,"),
		sysLine("answer any question it asks by typing; use /stop to cut to the draft."),
		sysLine("Tip: type / for the command palette (↑/↓ select · Tab complete · Enter run); ↑/↓ on an empty line recalls history."),
		sysLine("Sessions auto-save — if BINNY is interrupted, re-open the project (marked ⏵ resumable) to pick up where you left off."),
	}
}

func modelsLines(m *Model) []line {
	out := []line{sysLine("Configured providers (BYOK — set a key with /key, or its env var):")}
	cfg := m.sess.Config
	ids := make([]string, 0, len(cfg.Providers))
	for _, p := range cfg.Providers {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := cfg.Find(id)
		mark := styleErr.Render("no key")
		if p.HasKey() {
			mark = styleOK.Render("ready")
		}
		active := "  "
		if m.sess.Provider != nil && m.sess.Provider.ID == p.ID {
			active = styleChipHi.Render("▸ ")
		}
		out = append(out, sysLine(fmt.Sprintf("%s%-10s [%s] %s  models: %s",
			active, p.ID, p.Kind, mark, strings.Join(p.Models, ", "))))
		if d := config.DefaultBaseURL(p.ID); p.BaseURL != "" && p.BaseURL != d {
			out = append(out, sysLine(fmt.Sprintf("  %-10s endpoint: %s", "", p.BaseURL)))
		}
	}
	out = append(out, sysLine("Switch with /model <id>, or just /model to pick with ↑/↓."))
	return out
}

func statusLines(m *Model) []line {
	s := m.sess
	proj := "(none — /project to pick)"
	if s.Project != nil {
		proj = fmt.Sprintf("%s [%s, %s]", s.Project.Name, s.Project.Kind, s.Project.Status)
	}
	out := []line{
		sysLine("BINNY session:"),
		sysLine("  provider/model : " + providerID(m) + " / " + s.Model),
		sysLine("  mode           : " + modeLabel(s.Mode)),
		sysLine("  phase          : " + string(s.Phase)),
		sysLine("  project        : " + proj),
	}
	if s.Sample != nil {
		out = append(out, sysLine(fmt.Sprintf("  active sample  : %s (extra sample — /switch primary to go back)", s.Sample.DisplayName())))
	}
	return out
}
