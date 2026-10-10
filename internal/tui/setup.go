package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"openbin.ai/binny/internal/config"
	"openbin.ai/binny/internal/llm"
)

// The setup screen configures a BYOK LLM key on first run (and via /key). Keys
// are saved to ~/.config/binny/config.json (0600). Stage 0 picks a provider;
// stage 1 accepts the key (masked) and persists it; stage 2 picks the model
// from the provider's live-discovered list (dynamic, not hardcoded).

// modelsFetchedMsg carries a live GET /models result back to the UI thread.
type modelsFetchedMsg struct {
	provider string
	models   []string
	live     bool
}

// openKeySetup enters the setup screen, returning to `ret` when finished. With
// a preselected non-Bedrock provider it jumps straight to key entry (you
// already picked it); otherwise it opens the up/down provider picker.
func (m *Model) openKeySetup(ret screen, preselect string) {
	m.screen = screenSetup
	m.setupReturn = ret
	m.setupCursor = 0
	if preselect != "" {
		if p := m.sess.Config.Find(preselect); p != nil && p.Kind != config.KindBedrock {
			m.setupProvider = preselect
			m.setupStage = 1
			m.keyInput.SetValue("")
			m.keyInput.Focus()
			return
		}
		for i := range m.sess.Config.Providers {
			if m.sess.Config.Providers[i].ID == preselect {
				m.setupCursor = i
				break
			}
		}
	}
	m.setupStage = 0
}

// enterModelPick moves to stage 2 for the given provider: it becomes the active
// provider, the model list is seeded from config (so something shows instantly)
// and a live GET /models fetch is dispatched to refresh it.
func (m *Model) enterModelPick(providerID string) tea.Cmd {
	_ = m.sess.SetProvider(providerID)
	p := m.sess.Config.Find(providerID)
	if p == nil || p.Kind == config.KindBedrock {
		// Bedrock has no queryable per-account list — just take the default.
		m.append(okLine(fmt.Sprintf("Now using %s / %s.", providerID, m.sess.Model)))
		return m.finishSetup()
	}
	m.setupProvider = providerID
	m.setupStage = 2
	m.setupModels = append([]string(nil), p.Models...)
	m.setupModelsLive = false
	m.setupModelsBusy = true
	m.setupModelCursor = indexOf(m.setupModels, m.sess.Model)
	return fetchModelsCmd(*p)
}

// fetchModelsCmd runs live model discovery off the UI thread.
func fetchModelsCmd(p config.Provider) tea.Cmd {
	return func() tea.Msg {
		models, live := llm.ListModels(context.Background(), p)
		return modelsFetchedMsg{provider: p.ID, models: models, live: live}
	}
}

// handleModelsFetched folds a live model list into the stage-2 picker.
func (m *Model) handleModelsFetched(msg modelsFetchedMsg) (tea.Model, tea.Cmd) {
	// Ignore a stale fetch (user already moved on to another provider/screen).
	if m.screen != screenSetup || m.setupStage != 2 || msg.provider != m.setupProvider {
		return m, nil
	}
	m.setupModelsBusy = false
	if len(msg.models) > 0 {
		keep := m.currentModel()
		m.setupModels = msg.models
		m.setupModelsLive = msg.live
		m.setupModelCursor = indexOf(m.setupModels, keep)
	}
	return m, nil
}

func (m *Model) currentModel() string {
	if m.setupModelCursor >= 0 && m.setupModelCursor < len(m.setupModels) {
		return m.setupModels[m.setupModelCursor]
	}
	return m.sess.Model
}

func (m *Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	providers := m.sess.Config.Providers
	switch m.setupStage {
	case 0:
		return m.updateSetupProvider(msg, providers)
	case 1:
		return m.updateSetupKey(msg)
	default:
		return m.updateSetupModel(msg)
	}
}

// stage 0: pick a provider
func (m *Model) updateSetupProvider(msg tea.KeyMsg, providers []config.Provider) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		// Cancel: return to wherever setup was opened from (REPL, Settings, or
		// the start menu). finishSetup routes correctly for each.
		return m, m.finishSetup()
	case "s":
		return m, m.finishSetup()
	case "up":
		if m.setupCursor > 0 {
			m.setupCursor--
		}
	case "down":
		if m.setupCursor < len(providers)-1 {
			m.setupCursor++
		}
	case "k":
		// Set / change the key for the highlighted provider (even if it
		// already has one — key rotation).
		p := providers[m.setupCursor]
		if p.Kind == config.KindBedrock {
			m.append(sysLine("bedrock uses your AWS credentials — no key to set."))
			return m, nil
		}
		m.setupProvider = p.ID
		m.setupStage = 1
		m.keyInput.SetValue("")
		m.keyInput.Focus()
		return m, textinput.Blink
	case "enter":
		p := providers[m.setupCursor]
		// Already usable (has a key, or Bedrock/AWS) → pick its model.
		if p.HasKey() {
			return m, m.enterModelPick(p.ID)
		}
		// Needs a key → go enter it.
		m.setupProvider = p.ID
		m.setupStage = 1
		m.keyInput.SetValue("")
		m.keyInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// stage 1: entering the key
func (m *Model) updateSetupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.setupStage = 0
		return m, nil
	case "enter":
		// A pasted key can carry a trailing newline or, if the clipboard held
		// extra content, whole extra lines. Keys contain no whitespace, so take
		// the first whitespace-delimited token — this discards paste noise.
		key := strings.TrimSpace(m.keyInput.Value())
		if f := strings.Fields(key); len(f) > 0 {
			key = f[0]
		}
		if key == "" {
			return m, nil
		}
		if err := m.sess.Config.SetKey(m.setupProvider, key); err != nil {
			m.append(errLine("Couldn't save key: " + err.Error()))
			return m, m.finishSetup()
		}
		m.keyInput.Blur()
		m.append(okLine(fmt.Sprintf("Saved key for %s. Pick a model:", m.setupProvider)))
		// Key is in hand → discover this provider's live models and pick one.
		return m, m.enterModelPick(m.setupProvider)
	}
	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)
	return m, cmd
}

// stage 2: pick a model from the live-discovered list
func (m *Model) updateSetupModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.setupStage = 0 // back to provider list
		return m, nil
	case "up":
		if m.setupModelCursor > 0 {
			m.setupModelCursor--
		}
	case "down":
		if m.setupModelCursor < len(m.setupModels)-1 {
			m.setupModelCursor++
		}
	case "r":
		// Manual refresh of the live list.
		if p := m.sess.Config.Find(m.setupProvider); p != nil && !m.setupModelsBusy {
			m.setupModelsBusy = true
			return m, fetchModelsCmd(*p)
		}
	case "enter":
		if len(m.setupModels) == 0 {
			return m, m.finishSetup()
		}
		model := m.setupModels[clamp(m.setupModelCursor, 0, len(m.setupModels)-1)]
		if err := m.sess.SetModel(model); err != nil {
			m.append(errLine(err.Error()))
			return m, m.finishSetup()
		}
		m.append(okLine(fmt.Sprintf("Now using %s / %s. Ready to go.", m.setupProvider, m.sess.Model)))
		return m, m.finishSetup()
	}
	return m, nil
}

// finishSetup leaves the setup screen for the return target, kicking off the
// project load if we're heading to the picker.
func (m *Model) finishSetup() tea.Cmd {
	m.keyInput.Blur()
	m.setupModelsBusy = false
	switch m.setupReturn {
	case screenREPL:
		m.screen = screenREPL
		m.input.Focus()
		return textinput.Blink
	case screenSettings:
		m.screen = screenSettings
		return nil
	default:
		m.screen = screenStart
		return nil
	}
}

func (m *Model) viewSetup() string {
	var b strings.Builder
	b.WriteString(banner() + "\n\n")

	switch m.setupStage {
	case 0:
		b.WriteString(styleHeader.Render("Set up an LLM provider (BYOK)") + "\n")
		b.WriteString(styleDim.Render("BINNY runs on your own API key — it's stored locally in ~/.config/binny/config.json.") + "\n")
		b.WriteString(styleDim.Render("↑/↓ move · enter select · k set/change key · s skip · q back") + "\n\n")
		for i, p := range m.sess.Config.Providers {
			cursor := "  "
			nameStyle := styleDim
			if i == m.setupCursor {
				cursor = styleChipHi.Render("▸ ")
				nameStyle = styleChipHi
			}
			status := styleErr.Render("no key")
			if p.HasKey() {
				status = styleOK.Render("ready")
			}
			hint := ""
			if p.Kind == config.KindBedrock {
				hint = " (AWS creds — no key)"
			} else if p.APIKeyEnv != "" {
				hint = styleDim.Render(" · env: $" + p.APIKeyEnv)
			}
			b.WriteString(fmt.Sprintf("%s%s  %s%s\n", cursor, nameStyle.Render(fmt.Sprintf("%-10s", p.ID)), status, hint))
		}
		return b.String()

	case 1:
		p := m.sess.Config.Find(m.setupProvider)
		b.WriteString(styleHeader.Render("Enter your "+m.setupProvider+" API key") + "\n")
		if p != nil && p.APIKeyEnv != "" {
			b.WriteString(styleDim.Render("(or press esc and set $"+p.APIKeyEnv+" in your environment instead)") + "\n")
		}
		b.WriteString(styleDim.Render("The key is stored locally (0600) and never sent anywhere but the provider.") + "\n\n")
		b.WriteString(m.keyInput.View() + "\n\n")
		b.WriteString(styleDim.Render("enter save · esc back") + "\n")
		return b.String()

	default: // stage 2: model pick
		source := styleDim.Render("configured")
		if m.setupModelsLive {
			source = styleOK.Render("live · from " + m.setupProvider + "/models")
		}
		if m.setupModelsBusy {
			source = styleDim.Render("discovering models…")
		}
		b.WriteString(styleHeader.Render("Pick a model for "+m.setupProvider) + "  " + source + "\n")
		b.WriteString(styleDim.Render("↑/↓ move · enter select · r refresh · esc back") + "\n\n")
		if len(m.setupModels) == 0 {
			b.WriteString(styleDim.Render("No models discovered. Press enter to use the provider default, or esc to go back.") + "\n")
			return b.String()
		}
		for i, id := range m.setupModels {
			cursor := "  "
			nameStyle := styleDim
			if i == m.setupModelCursor {
				cursor = styleChipHi.Render("▸ ")
				nameStyle = styleChipHi
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, nameStyle.Render(id)))
		}
		return b.String()
	}
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return 0
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
