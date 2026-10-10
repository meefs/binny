package tools

import (
	"strings"
	"testing"

	"openbin.ai/binny/internal/api"
)

// analysis with call graph: export "handle_request" → "parse" → "copy_field"
// (the sink). "helper" is an unrelated function with no path to the sink.
func reachAnalysis() *api.Analysis {
	fn := func(name string, callers, callees []string) api.Function {
		return api.Function{Name: name, Address: "0x0", Xrefs: api.Xrefs{Callers: callers, Callees: callees}}
	}
	return &api.Analysis{
		Exports: []api.Symbol{{Name: "handle_request"}},
		Functions: []api.Function{
			fn("handle_request", nil, []string{"parse"}),
			fn("parse", []string{"handle_request"}, []string{"copy_field"}),
			fn("copy_field", []string{"parse"}, nil), // the sink
			fn("helper", nil, nil),                   // isolated
		},
	}
}

func TestReachabilityFindsPath(t *testing.T) {
	out := traceReachability(reachAnalysis(), "copy_field", nil, 15)
	if !strings.HasPrefix(out, "REACHABLE") {
		t.Fatalf("expected reachable, got: %s", out)
	}
	if !strings.Contains(out, "handle_request → parse → copy_field") {
		t.Errorf("expected the source→sink path, got: %s", out)
	}
}

func TestReachabilityNoPath(t *testing.T) {
	// helper has no callers and isn't a source → not reachable.
	out := traceReachability(reachAnalysis(), "helper", nil, 15)
	if !strings.HasPrefix(out, "NOT reachable") {
		t.Errorf("expected not reachable, got: %s", out)
	}
	// The under-approximation caveat must be present so a null result isn't read
	// as proof of safety.
	if !strings.Contains(out, "under-approximate") && !strings.Contains(out, "UNDER-approximate") {
		t.Errorf("expected the indirect-call caveat, got: %s", out)
	}
}

func TestReachabilitySinkIsSource(t *testing.T) {
	out := traceReachability(reachAnalysis(), "handle_request", nil, 15)
	if !strings.Contains(out, "itself a source") {
		t.Errorf("an exported sink should report as directly reachable, got: %s", out)
	}
}

func TestReachabilityDepthLimit(t *testing.T) {
	// Depth 1 can't cross the two hops from the export to the sink.
	out := traceReachability(reachAnalysis(), "copy_field", nil, 1)
	if !strings.HasPrefix(out, "NOT reachable") {
		t.Errorf("depth 1 should not reach a 2-hop source, got: %s", out)
	}
}

func TestReachabilityCustomSources(t *testing.T) {
	// Restrict sources to "parse": the sink is one hop from it → reachable.
	out := traceReachability(reachAnalysis(), "copy_field", []string{"parse"}, 15)
	if !strings.HasPrefix(out, "REACHABLE") || !strings.Contains(out, "parse → copy_field") {
		t.Errorf("expected reachable from custom source parse, got: %s", out)
	}
}

func TestReachabilitySinkNotFound(t *testing.T) {
	out := traceReachability(reachAnalysis(), "nope", nil, 15)
	if !strings.Contains(out, "not found") {
		t.Errorf("expected not-found message, got: %s", out)
	}
}
