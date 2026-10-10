package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"openbin.ai/binny/internal/agent"
	"openbin.ai/binny/internal/api"
	"openbin.ai/binny/internal/tools"
)

// Multi-sample projects: /samples lists the project's extra samples and
// /switch re-points BINNY's read tools at one of them (or back to the
// primary). Both do their HTTP off the UI thread and report back via
// samplesMsg. The switch itself happens in the toolbox (same hot-swap as
// decompile_sample) when an engine exists; before the engine is built it's
// just recorded on the session and Build loads it.

type samplesMsg struct {
	lines []line
	// switched is set when a /switch succeeded: the new active sample (nil =
	// primary) to record on the session.
	switched bool
	target   *api.Sample
}

// cmdSamples lists the project's samples (async).
func (m *Model) cmdSamples() tea.Cmd {
	p := m.sess.Project
	if p == nil {
		m.append(errLine("No project selected — /project first."))
		return nil
	}
	if p.IsLocal() || p.Kind != api.KindBIN {
		m.append(sysLine("Samples only apply to backend BIN projects — this one holds a single binary."))
		return nil
	}
	active := m.sess.Sample
	client := m.sess.API
	return func() tea.Msg {
		samples, err := client.ListSamples(p.ID)
		if err != nil {
			return samplesMsg{lines: []line{errLine("Couldn't list samples: " + err.Error())}}
		}
		var out []line
		for _, l := range strings.Split(strings.TrimRight(agent.FormatSamples(p, samples, active), "\n"), "\n") {
			out = append(out, sysLine(l))
		}
		if len(samples) > 0 {
			out = append(out, sysLine("/switch <#|label|primary> to change which one BINNY reads."))
		}
		return samplesMsg{lines: out}
	}
}

// cmdSwitch re-points the read tools at another sample (async).
func (m *Model) cmdSwitch(query string) tea.Cmd {
	p := m.sess.Project
	if p == nil {
		m.append(errLine("No project selected — /project first."))
		return nil
	}
	if p.IsLocal() || p.Kind != api.KindBIN {
		m.append(sysLine("Samples only apply to backend BIN projects — nothing to switch."))
		return nil
	}
	if strings.TrimSpace(query) == "" {
		m.append(sysLine("Usage: /switch <#|label|primary> — /samples lists them."))
		return nil
	}
	var tb *tools.Toolbox
	if m.engine != nil {
		tb = m.engine.Toolbox()
	}
	client := m.sess.API
	return func() tea.Msg {
		samples, err := client.ListSamples(p.ID)
		if err != nil {
			return samplesMsg{lines: []line{errLine("Couldn't list samples: " + err.Error())}}
		}
		target, err := tools.ResolveSample(samples, query)
		if err != nil {
			return samplesMsg{lines: []line{errLine(err.Error())}}
		}
		if tb == nil {
			// No engine yet: Build picks this up and loads the blob.
			if target != nil && !target.Ready() {
				return samplesMsg{lines: []line{errLine(fmt.Sprintf("sample %q is %s — only READY samples have an analysis", target.DisplayName(), target.Status))}}
			}
			return samplesMsg{switched: true, target: target,
				lines: []line{okLine(agent.SwitchSummary(p, target, nil))}}
		}
		an, err := tb.SwitchSample(target)
		if err != nil {
			return samplesMsg{lines: []line{errLine("Switch failed: " + err.Error())}}
		}
		return samplesMsg{switched: true, target: target,
			lines: []line{okLine(agent.SwitchSummary(p, target, an))}}
	}
}

func (m *Model) handleSamplesMsg(msg samplesMsg) (tea.Model, tea.Cmd) {
	m.append(msg.lines...)
	if msg.switched {
		m.sess.Sample = msg.target
		if m.engineStarted {
			m.append(sysLine("BINNY sees the new sample on its next tool call — tell it what to look at (e.g. \"triage the payload now\")."))
		}
		m.persist()
	}
	return m, nil
}
