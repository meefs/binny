package agent

import (
	"openbin.ai/binny/internal/app"
	"openbin.ai/binny/internal/tools"
)

// registerExtractionTools adds the active-extraction toolset (write_file / run
// / read_artifact) — the sandboxed config-extraction capability.
//
// The tools are ALWAYS registered, even with no sample attached yet: the
// sandbox reads the sample path lazily from the session, so the analyst can
// attach it mid-run with /sample <path>. Without one, the tools return an
// actionable error telling BINNY to ask the analyst to attach the binary —
// rather than the capability silently not existing (which just made BINNY
// describe crypto instead of extracting it).
func registerExtractionTools(reg *tools.Registry, inv *Investigation, sess *app.Session) {
	registerSandboxTools(reg, inv, sess)
}
