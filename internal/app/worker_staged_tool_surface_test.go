package app

// THE TOOLS A SPAWNED WORKER IS ACTUALLY GIVEN (nocx-xn63t.6.49).
//
// What the owner saw in their stand is the whole of this file's reason: a
// worker launched through `workers.spawn {command: claude}` ran with argv
// `claude` and no `--mcp-config`, so it could not see `mcp__nocx__workers_report`
// — its ToolSearch answered "no matching deferred tools found" — and its
// coordinator read its screen instead of receiving its report.
//
// The unit of the defect is a classification: the shell asked ONE field
// (`local`) both "does a saved record apply" and "is there a tool surface",
// and a worker's pane answers no and yes. The shell-level half is
// agent_enrol_shell_test.go's (a real pty, a real wrapper). THIS file is the
// half no shell test can reach: that the launch a coordinator really starts —
// through workers.spawn, the real helper, the real endpoint and the shipped
// authorizer — ends with the worker discovering and using its own tools, and
// with its coordinator holding the report.
//
// # What is real here
//
// EVERY HOP. The stand is session_surface_realhelper_test.go's: a real
// cmd/nocx-helper daemon, a real pty, a real shell, the real local spawn path,
// the real tool endpoint, the real authorizer, dispatcher and worker record.
// The launch is resolved by the PRODUCT's own resolver (agentLaunchService),
// wired as app.go wires it — including its worker-lane classification, which
// is why the record below is one nothing may run from. The tool calls are made
// by the mock agent in the worker's pane through the shipped MCP bridge
// (`<helper> mcp --socket ...`), started from the config file the shell
// integration STAGED, so the process tree the endpoint admits is the worker
// pane's own.
//
// # What is substituted, and what that leaves unproven
//
// The AGENT is a mock (internal/app/testdata/s14mock), as it is in every check
// on this path: the thing being orchestrated is faked, the orchestration is
// not. It cannot decide on its own to report, so the call is cued by file —
// what this file therefore does NOT claim is that a real model reaches for
// `workers.report` unprompted.
//
// THE STAND'S OWN mcp.json IS DELIBERATELY NOT WRITTEN. The mock prefers the
// config its launch names and falls back to that file, so leaving it absent is
// what turns "the worker could call a tool" into "the worker could call a tool
// BECAUSE its pane staged a surface": with no staging there is no config, the
// mock says `NO-MCP-CONFIG`, and the catalogue below fails rather than passing
// for a reason this check did not create.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agentrecord"
	"github.com/shady2k/nocx/internal/waittest"
	"github.com/shady2k/nocx/internal/workers"
)

// launchRecord is one launch as the program in the pane recorded it: the
// arguments it was started with, and the config file it took its bridge paths
// from.
type launchRecord struct {
	Argv   []string `json:"argv"`
	Staged bool     `json:"staged"`
	Config string   `json:"config"`
}

// waitForLaunch waits for the mock's own record of its launch, and refuses a
// record that never arrives rather than reading a zero value as one.
func (s *s14RealStand) waitForLaunch(t *testing.T, sessionID string) launchRecord {
	t.Helper()
	var record launchRecord
	path := filepath.Join(s.stateDir, "launch", sessionID)
	waittest.WaitForDetail(t, fmt.Sprintf("the launch record for %s", sessionID), func() string {
		raw, err := os.ReadFile(path) // #nosec G304 -- this test's own state directory
		if err != nil {
			return fmt.Sprintf("reading %s: %v", path, err)
		}
		return fmt.Sprintf("reading %s: %q", path, raw)
	}, func() bool {
		raw, err := os.ReadFile(path) // #nosec G304 -- this test's own state directory
		if err != nil {
			return false
		}
		return json.Unmarshal(raw, &record) == nil
	})
	return record
}

func TestASpawnedWorkerFindsItsToolsThroughTheSurfaceItsPaneStaged(t *testing.T) {
	mockDir := s14RealBuildMock(t)
	stateDir := t.TempDir()
	capturesDir, err := filepath.Abs(filepath.Join("..", "agentdriver", "testdata", "captures"))
	if err != nil {
		t.Fatalf("resolve captures dir: %v", err)
	}
	// Set BEFORE the stand: the real daemon it starts inherits this process's
	// environment at that moment, and every shell it forks — the worker's own
	// pane included — inherits the daemon's. `claude` is the mock and nothing
	// else: no launcher stands between the wrapper and the program it started.
	t.Setenv("S14_CAPTURES_DIR", capturesDir)
	t.Setenv("S14_STATE_DIR", stateDir)
	t.Setenv("PATH", mockDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	records, recordsErr := agentrecord.New(t.TempDir())
	if recordsErr != nil {
		t.Fatalf("agent record store: %v", recordsErr)
	}
	// A RECORD NOTHING MAY RUN FROM. It is the decoy the whole fix is about: a
	// worker's pane is a local pane, so this store is in the resolver's hands,
	// and the command below is one that does not exist. If the record reached
	// the worker, nothing would start and the first call in this test would
	// fail — the negative half is asserted by construction rather than
	// assumed from the positive one.
	if saveErr := records.Save("claude", agentrecord.Document{
		Command: filepath.Join(t.TempDir(), "record-claude"),
		Args:    []string{"--record-only"},
		Env:     []string{"NOCX_RECORD_ONLY=1"},
	}); saveErr != nil {
		t.Fatalf("save the decoy record: %v", saveErr)
	}

	stand := newS14RealStand(t, mockDir, stateDir, withStagedWorkerLaunch(records), withNoAdmissionRoot())
	coordinator := workerWorkerSetup{coordinator: stand.coord.ID(), dispatcher: stand.dispatcher, record: stand.record}

	const task = "count the frames in the capture and report"
	spawned := mustDispatch(t, coordinator, "workers.spawn", `{"command":"claude","task":"`+task+`"}`)
	var spawnResult struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(spawned, &spawnResult); err != nil {
		t.Fatalf("decode workers.spawn result %s: %v", spawned, err)
	}
	if spawnResult.ID == "" || spawnResult.State != string(workers.StateLive) {
		t.Fatalf("workers.spawn = %s, want a live participant id", spawned)
	}
	// THE WORKER'S PANE IS UP, which is already the first half: production's
	// own spawner waits for the helper's emulator to classify that pane before
	// it answers live, so a worker that never ran could not be reported here.
	workerSession := stand.liveSession(t, spawnResult.ID)

	// ── THE OWNER'S OWN EVIDENCE, asserted rather than inferred. ───────────
	//
	// The report was "argv `claude` and no --mcp-config", so this check reads
	// the arguments the program in that pane was actually started with, and
	// the config file it took its bridge paths from. Both matter: an argument
	// naming a file nobody can read would be the same defect wearing a flag.
	launch := stand.waitForLaunch(t, workerSession)
	if !launch.Staged {
		t.Fatalf("the worker's launch was started with %v and no staged config; the surface its pane offers never reached it", launch.Argv)
	}
	stagedFlag := ""
	for i := 0; i+1 < len(launch.Argv); i++ {
		if launch.Argv[i] == "--mcp-config" {
			stagedFlag = launch.Argv[i+1]
		}
	}
	if stagedFlag == "" {
		t.Fatalf("the worker's argv is %v, want the staged MCP config appended", launch.Argv)
	}
	if !strings.Contains(stagedFlag, "/nocx-agent-launch.") || !strings.HasSuffix(stagedFlag, "/mcp.json") {
		t.Fatalf("the worker was pointed at %q, want the private directory the wrapper staged", stagedFlag)
	}
	if launch.Config != stagedFlag {
		t.Fatalf("the worker read its bridge paths from %q and its argv named %q", launch.Config, stagedFlag)
	}
	if len(launch.Argv) != 2 {
		t.Fatalf("the worker's argv is %v, want the typed command's own arguments and the staged config and nothing else", launch.Argv)
	}

	// ── DISCOVERY: what the worker's own agent can see it may do. ──────────
	//
	// A tools/list through the bridge, over the endpoint, as the worker's own
	// pane — the call an agent makes before it uses anything. The vocabulary
	// is read here rather than described, and both halves are asserted: the
	// tool this bead was filed about is present, and a coordinator's own
	// `workers.spawn` is NOT, because the grant a worker's pane is admitted
	// with is the worker's.
	catalogue := stand.cueCatalogue(t, workerSession)
	if _, ok := catalogue["workers.report"]; !ok {
		t.Fatalf("the worker cannot see workers.report; its catalogue is %v", s14Names(catalogue))
	}
	if _, ok := catalogue["workers.spawn"]; ok {
		t.Fatalf("the worker was offered a coordinator's tool; its catalogue is %v", s14Names(catalogue))
	}

	// ── USE: the report itself, and the coordinator's own mailbox. ────────
	const words = "3 frames counted; the third is the light one"
	answer := s14ToolResult(t, "workers.report",
		stand.cueAnswer(t, workerSession, "workers.report", map[string]any{"kind": "done", "text": words}),
		s14BridgeTree(stateDir))
	var committed struct {
		ID  string `json:"id"`
		Seq int64  `json:"seq"`
	}
	if err := json.Unmarshal(answer, &committed); err != nil {
		t.Fatalf("decode the report result: %v", err)
	}
	if committed.ID == "" || committed.Seq == 0 {
		t.Fatalf("the worker's report answered with no committed row: %s", answer)
	}

	inbox := readOwnInbox(t, coordinator)
	messages := *inbox.Messages
	if len(messages) != 1 {
		t.Fatalf("the coordinator was handed %d messages, want the worker's one report: %+v", len(messages), messages)
	}
	if messages[0].Message != words || messages[0].Kind != string(workers.KindDone) {
		t.Fatalf("the coordinator's mail = %+v, want the worker's own words %q as a done report", messages[0], words)
	}
	if strings.TrimSpace(messages[0].From) == "" {
		t.Fatalf("the report reached the mailbox with no sender, so nothing says which worker spoke: %+v", messages[0])
	}
}
