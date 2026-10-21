package library

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// a library timeout is seconds; omp's is milliseconds (discovery/mcp-json.ts),
// so it is written ×1000 and read back ÷1000
func TestOmpTimeoutIsMilliseconds(t *testing.T) {
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 45}
	omp := &mcpFile{Format: fmtOmp}
	if v, ok := valueOf(omp.encode(s), "timeout"); !ok || v != 45000 {
		t.Fatalf("omp timeout = %v %v, want 45000ms", v, ok)
	}
	got, ok := omp.decode("fs", map[string]any{"command": "npx", "timeout": float64(45000)})
	if !ok || got.Timeout != 45 {
		t.Fatalf("read back %+v", got)
	}
}

// no library timeout writes no omp timeout, leaving omp's own 30 seconds
func TestOmpTimeoutLeftOutWhenUnset(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx"}
	if _, ok := valueOf(omp.encode(s), "timeout"); ok {
		t.Fatal("a timeout was written for a server that set none")
	}
}

// a library timeout is magpie's over the user's old value; an unset one
// leaves the user's timeout exactly as it was
func TestLibraryTimeoutOverridesUserTimeout(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	old := map[string]any{"timeout": float64(30000)}
	set := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 90}
	if v, _ := valueOf(omp.merged(set, old), "timeout"); v != 90000 {
		t.Errorf("library timeout lost to the user's: %v", v)
	}
	unset := &Server{Name: "fs", Transport: "stdio", Command: "npx"}
	if v, _ := valueOf(omp.merged(unset, old), "timeout"); v != float64(30000) {
		t.Errorf("user's timeout changed without a library one: %v", v)
	}
}

// Codex's tool timeout round-trips, and a library timeout beats the user's
// old tool_timeout_sec
func TestCodexTimeoutRoundTrips(t *testing.T) {
	codex := &mcpFile{Format: fmtCodex}
	s := &Server{Name: "x", Transport: "stdio", Command: "x", Timeout: 120}
	if v, ok := valueOf(codex.encode(s), "tool_timeout_sec"); !ok || v != 120 {
		t.Fatalf("codex tool_timeout_sec = %v %v", v, ok)
	}
	if v, _ := valueOf(codex.merged(s, map[string]any{"tool_timeout_sec": 30}), "tool_timeout_sec"); v != 120 {
		t.Errorf("library timeout lost to the user's 30: %v", v)
	}
	back, _ := codex.decode("x", map[string]any{"command": "x", "tool_timeout_sec": 120})
	if back.Timeout != 120 {
		t.Errorf("read back %d, want 120", back.Timeout)
	}
}

// Goose's 300 is the default magpie gives every server, not one the library
// set, so it reads back as no timeout
func TestGooseDefaultIsNotALibraryTimeout(t *testing.T) {
	goose := &mcpFile{Format: fmtGoose}
	back, _ := goose.decode("x", map[string]any{"type": "stdio", "cmd": "x", "timeout": gooseOldTimeout})
	if back.Timeout != 0 {
		t.Errorf("Goose's default read back as %d", back.Timeout)
	}
}

// the library's comparison: an unset timeout matches whatever the user set,
// so syncing leaves the entry alone; a set one is compared for real
func TestCurrentComparesTimeoutOnlyWhenTheLibrarySetsOne(t *testing.T) {
	omp := &mcpFile{Format: fmtOmp}
	m := map[string]any{"command": "npx", "timeout": float64(30000)}
	unset := &Server{Name: "fs", Transport: "stdio", Command: "npx"}
	if cur, _ := omp.current("fs", m, unset); !cur.same(unset) {
		t.Error("a user timeout made an unset server look out of sync")
	}
	set := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 90}
	if cur, _ := omp.current("fs", m, set); cur.same(set) {
		t.Error("a library timeout of 90 matched the user's 30")
	}
}

// magpie-image's own tool timeout is magpie's default: it doesn't read back
// as a library timeout, so the self server stays in sync after an upgrade
func TestSelfServerDefaultIsNotALibraryTimeout(t *testing.T) {
	codex := &mcpFile{Format: fmtCodex}
	self := &Server{Name: selfServerName, Transport: "stdio", Command: "/opt/magpie", Args: []string{"mcp", "image"}}
	back, _ := codex.decode(selfServerName, valueMap(codex.encode(self)))
	if back.Timeout != 0 {
		t.Errorf("magpie-image's default read back as a library timeout %d", back.Timeout)
	}
}

// validation: a negative timeout is refused on every save path (check)
func TestNegativeTimeoutIsRefused(t *testing.T) {
	s := Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: -1}
	if err := s.check(); err == nil {
		t.Fatal("a negative timeout was accepted")
	}
}

// the CLI's timeout reaches the saved entry, end to end
func TestSaveServerKeepsTimeoutInTheAgentFile(t *testing.T) {
	h := sandbox(t)
	p := filepath.Join(h, ".omp", "agent", "mcp.json")
	holdCLI(t, h, "omp") // omp with its MCP file alone is gone (#2177), so unwritten
	ok(t)(SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 45, Agents: []string{"omp"}}))
	var file struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(read(t, p)), &file); err != nil {
		t.Fatalf("omp's mcp.json is not JSON: %v", err)
	}
	if got := file.MCPServers["fs"]["timeout"]; got != float64(45000) {
		t.Fatalf("omp's timeout = %v, want 45000 ms", got)
	}
}

// valueMap turns an ordered entry into a plain map, as an agent file reads.
func valueMap(o ordered) map[string]any {
	m := map[string]any{}
	for _, e := range o {
		m[e.k] = e.v
	}
	return m
}

// Gemini's timeout is milliseconds and Hermes' is seconds, and both
// round-trip; Hermes' connect_timeout is its startup one, not the request
// timeout. Alma is not verified to read a timeout, so none is written.
func TestGeminiAndHermesTimeoutsRoundTrip(t *testing.T) {
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 45}
	gem := &mcpFile{Format: fmtGemini}
	if v, ok := valueOf(gem.encode(s), "timeout"); !ok || v != 45000 {
		t.Fatalf("gemini timeout = %v %v, want 45000 ms", v, ok)
	}
	if back, _ := gem.decode("fs", map[string]any{"command": "npx", "timeout": float64(45000)}); back.Timeout != 45 {
		t.Errorf("gemini read back %d, want 45", back.Timeout)
	}
	hermes := &mcpFile{Format: fmtHermes}
	if v, ok := valueOf(hermes.encode(s), "timeout"); !ok || v != 45 {
		t.Fatalf("hermes timeout = %v %v, want 45 seconds", v, ok)
	}
	if back, _ := hermes.decode("fs", map[string]any{"command": "npx", "timeout": float64(45), "connect_timeout": float64(60)}); back.Timeout != 45 {
		t.Errorf("hermes read back %d, want 45", back.Timeout)
	}
	if _, ok := valueOf((&mcpFile{Format: fmtAlma}).encode(s), "timeout"); ok {
		t.Error("alma wrote a timeout it isn't verified to read")
	}
}

// a format with no timeout key (OpenCode's timeout is for fetching tools, not
// calls) writes none, and compares as the same server, so the entry isn't
// rewritten on every sync
func TestTimeoutAnAgentCannotTakeIsNotCompared(t *testing.T) {
	oc := &mcpFile{Format: fmtOpenCode}
	s := &Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 60}
	if oc.carriesTimeout() {
		t.Fatal("opencode is said to carry a timeout")
	}
	if _, ok := valueOf(oc.encode(s), "timeout"); ok {
		t.Error("opencode wrote a timeout")
	}
	m := map[string]any{"type": "local", "command": []any{"npx"}}
	if cur, _ := oc.current("fs", m, s); !cur.same(s) {
		t.Error("an agent that can't take a timeout compares as out of sync")
	}
}

// a timeout an agent's format can't take is reported by the sync that gives
// it the server; the server is still written to it
func TestTimeoutAnAgentCannotTakeIsReported(t *testing.T) {
	sandbox(t)
	// the report is a problem, not an error: the save itself succeeds
	res, err := SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 60, Agents: []string{"opencode"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Problems {
		if p.Agent == "opencode" && p.What == "mcp:fs" && strings.Contains(p.Error, "60 seconds") {
			return
		}
	}
	t.Fatalf("the 60 seconds opencode can't take weren't reported: %+v", res.Problems)
}

// serverTimeout is the timeout of the named server in an agent's JSON file
// (mcpServers), nil when it has none.
func serverTimeout(t *testing.T, path, name string) any {
	t.Helper()
	var file struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(read(t, path)), &file); err != nil {
		t.Fatalf("%s is not JSON: %v", path, err)
	}
	return file.MCPServers[name]["timeout"]
}

// a save that sends no timeout (the GUI's form has none) keeps the server's
// through an edit, a rename and a new agent given it; an explicit 0 resets it
func TestSaveWithoutTimeoutKeepsIt(t *testing.T) {
	h := sandbox(t)
	ompFile := filepath.Join(h, ".omp", "agent", "mcp.json")
	gemFile := filepath.Join(h, ".gemini", "settings.json")
	holdCLI(t, h, "omp") // omp with its MCP file alone is gone (#2177), so unwritten
	ok(t)(SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "npx", Timeout: 60, Agents: []string{"omp"}}))
	ok(t)(SaveServerKeepTimeout("fs", Server{Name: "fs", Transport: "stdio", Command: "npx", Agents: []string{"omp"}}))
	ok(t)(SaveServerKeepTimeout("fs", Server{Name: "fs2", Transport: "stdio", Command: "npx", Agents: []string{"omp"}}))
	ok(t)(ServerAgents("fs2", []string{"omp", "gemini"}))
	l, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if s := l.server("fs2"); s == nil || s.Timeout != 60 {
		t.Fatalf("the library's fs2 = %+v, want its 60 seconds kept", s)
	}
	if got := serverTimeout(t, ompFile, "fs2"); got != float64(60000) {
		t.Fatalf("omp's fs2 timeout = %v, want 60000 ms", got)
	}
	if got := serverTimeout(t, gemFile, "fs2"); got != float64(60000) {
		t.Fatalf("gemini's fs2 timeout = %v, want 60000 ms", got)
	}
	ok(t)(SaveServer("fs2", Server{Name: "fs2", Transport: "stdio", Command: "npx", Agents: []string{"omp"}}))
	if l, err = load(); err != nil {
		t.Fatal(err)
	}
	if s := l.server("fs2"); s == nil || s.Timeout != 0 {
		t.Fatalf("an explicit 0 didn't reset fs2: %+v", s)
	}
	// the reset leaves omp's file JSON; the 60000 magpie wrote stays in its
	// entry, as an unset timeout leaves what the agent's file has
	serverTimeout(t, ompFile, "fs2")
}

// the request timeout is the library's to set; a startup timeout the user set
// (Codex's startup_timeout_sec, Hermes' connect_timeout) is theirs. A library
// timeout replaces only the request one, and an unset one leaves both.
func TestTimeoutMatrixKeepsStartupTimeouts(t *testing.T) {
	cases := []struct {
		name    string
		format  mcpFormat
		req     string
		unit    int // the request key's value for one second
		startup string
	}{
		{"omp", fmtOmp, "timeout", 1000, ""},
		{"gemini", fmtGemini, "timeout", 1000, ""},
		{"hermes", fmtHermes, "timeout", 1, "connect_timeout"},
		{"codex", fmtCodex, "tool_timeout_sec", 1, "startup_timeout_sec"},
		{"goose", fmtGoose, "timeout", 1, ""},
	}
	for _, c := range cases {
		f := &mcpFile{Format: c.format}
		old := map[string]any{c.req: float64(30 * c.unit)}
		if c.startup != "" {
			old[c.startup] = float64(20)
		}
		set := &Server{Name: "x", Transport: "stdio", Command: "x", Timeout: 90}
		merged := f.merged(set, old)
		if v, _ := valueOf(merged, c.req); v != 90*c.unit {
			t.Errorf("%s: request timeout = %v, want %d", c.name, v, 90*c.unit)
		}
		if c.startup != "" {
			if v, _ := valueOf(merged, c.startup); v != float64(20) {
				t.Errorf("%s: the user's startup timeout came out %v, want 20", c.name, v)
			}
		}
		if back, ok := f.decode("x", valueMap(merged)); !ok || back.Timeout != 90 {
			t.Errorf("%s: read back %+v, want a request timeout of 90", c.name, back)
		}
		unset := &Server{Name: "x", Transport: "stdio", Command: "x"}
		if c.startup != "" {
			if v, _ := valueOf(f.merged(unset, old), c.startup); v != float64(20) {
				t.Errorf("%s: an unset library timeout lost the startup one: %v", c.name, v)
			}
		}
	}
}
