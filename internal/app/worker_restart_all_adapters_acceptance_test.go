package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/shady2k/nocx/internal/agentrecord"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/storage"
	"github.com/shady2k/nocx/internal/storage/storagetest"
	"github.com/shady2k/nocx/internal/waittest"
	"github.com/shady2k/nocx/internal/workers"
)

// TestAppRestartRestoresEveryRecordedAgentInItsPaneAndRefusesALazyID is the
// stage acceptance across the real app composition root. It writes the same
// durable worker-record shape the spawner owns, starts a fresh App over those
// documents, and observes the actual CLI argv, working directory, pane
// enrolment, visible layout rows, and the lazy-ID refusal on the restored
// panes. The fake CLIs replace only the external agents; sessions, shells,
// shell integration, helper, layout and restart are production code.
func TestAppRestartRestoresEveryRecordedAgentInItsPaneAndRefusesALazyID(t *testing.T) {
	artifacts := realHelperArtifacts(t)
	home := storagetest.IsolateWithHome(t)
	helperBinary := filepath.Join(helperRoot(home, artifacts.hash()), "nocx-helper")
	t.Cleanup(func() { endTheDaemon(t, helperBinary) })

	invocations := t.TempDir()
	fakeBin := t.TempDir()
	for _, name := range []string{"claude", "codex", "omp", "prime-agent"} {
		writeRestartFakeCLI(t, filepath.Join(fakeBin, name), invocations)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NOCX_RESTART_LOG_DIR", invocations)

	paths, pathsErr := storage.NewAppPaths()
	if pathsErr != nil {
		t.Fatalf("app paths: %v", pathsErr)
	}
	records, recordsErr := agentrecord.New(paths.ConfigDir())
	if recordsErr != nil {
		t.Fatalf("agent record store: %v", recordsErr)
	}
	for _, agent := range []string{"codex", "omp", "prime-agent"} {
		resumeIDArgs := map[string][]string{
			"codex":       {"resume", "{UUID}"},
			"omp":         {"--resume={UUID}"},
			"prime-agent": {"-r", "{UUID}"},
		}[agent]
		doc := agentrecord.Document{
			DisplayName: agent,
			Command:     agent,
			Resume: agentrecord.Resume{
				SessionIDArgs: []string{"--session-id", "{UUID}"},
				ResumeIDArgs:  resumeIDArgs,
				ResumeCwdArgs: map[string][]string{
					"codex":       {"resume", "--last"},
					"omp":         {"--continue"},
					"prime-agent": {"-c"},
				}[agent],
			},
		}
		if saveErr := records.Save(agent, doc); saveErr != nil {
			t.Fatalf("save %s agent record: %v", agent, saveErr)
		}
	}

	worktrees := createRestartAcceptanceWorktrees(t)
	agents := []struct {
		name, id, tab, pane, cwd, branch string
		mode                             workers.ResumeMode
		args                             []string
		hasWorktree, lazy                bool
	}{
		// The worktree branch exercises the location-selected cwd form.
		{name: "claude", branch: "restart-acceptance-claude", tab: "00000000-0000-7000-8000-00000000f401", pane: "00000000-0000-7000-8000-00000000f411", cwd: worktrees[0], mode: workers.ResumeByCwd, args: []string{"--continue"}, hasWorktree: true},
		{name: "codex", branch: "restart-acceptance-codex", tab: "00000000-0000-7000-8000-00000000f402", pane: "00000000-0000-7000-8000-00000000f412", cwd: worktrees[1], mode: workers.ResumeByCwd, args: []string{"resume", "--last"}, hasWorktree: true},
		{name: "omp", branch: "restart-acceptance-omp", tab: "00000000-0000-7000-8000-00000000f403", pane: "00000000-0000-7000-8000-00000000f413", cwd: worktrees[2], mode: workers.ResumeByCwd, args: []string{"--continue"}, hasWorktree: true},
		{name: "prime-agent", branch: "restart-acceptance-prime-agent", tab: "00000000-0000-7000-8000-00000000f404", pane: "00000000-0000-7000-8000-00000000f414", cwd: worktrees[3], mode: workers.ResumeByCwd, args: []string{"-c"}, hasWorktree: true},
		// The shared-checkout branch verifies each stable ID is sent to its own adapter.
		{name: "claude", id: "claude-session-1", tab: "00000000-0000-7000-8000-00000000f405", pane: "00000000-0000-7000-8000-00000000f415", cwd: worktrees[4], mode: workers.ResumeByID, args: []string{"--resume", "claude-session-1"}},
		{name: "codex", id: "codex-session-2", tab: "00000000-0000-7000-8000-00000000f406", pane: "00000000-0000-7000-8000-00000000f416", cwd: worktrees[4], mode: workers.ResumeByID, args: []string{"resume", "codex-session-2"}},
		{name: "omp", id: "omp-session-3", tab: "00000000-0000-7000-8000-00000000f407", pane: "00000000-0000-7000-8000-00000000f417", cwd: worktrees[4], mode: workers.ResumeByID, args: []string{"--resume=omp-session-3"}},
		{name: "prime-agent", id: "prime-session-4", tab: "00000000-0000-7000-8000-00000000f408", pane: "00000000-0000-7000-8000-00000000f418", cwd: worktrees[4], mode: workers.ResumeByID, args: []string{"-r", "prime-session-4"}},
		{name: "claude", tab: "00000000-0000-7000-8000-00000000f409", pane: "00000000-0000-7000-8000-00000000f419", cwd: worktrees[4], mode: workers.ResumeNone, lazy: true},
	}

	ctx := context.Background()
	first, firstAppErr := newTestApp(t, withLocalHelperArtifacts(artifacts))
	if firstAppErr != nil {
		t.Fatalf("New first app: %v", firstAppErr)
	}
	t.Cleanup(func() { first.Shutdown(context.Background()) })
	if firstStartErr := first.Start(ctx); firstStartErr != nil {
		t.Fatalf("Start first app: %v", firstStartErr)
	}
	conn := dialAppWS(t, first)
	state := callAppWS(t, conn, "layout.read", map[string]any{}, 1)
	if state.Error != nil {
		t.Fatalf("read initial layout: %+v", state.Error)
	}
	var layout struct {
		DefaultWorkspaceID string `json:"defaultWorkspaceId"`
	}
	if layoutDecodeErr := json.Unmarshal(state.Result, &layout); layoutDecodeErr != nil || layout.DefaultWorkspaceID == "" {
		t.Fatalf("initial layout = %s (err %v), want default workspace", state.Result, layoutDecodeErr)
	}

	priorSessions := make(map[string]session.ID, len(agents))
	for i, agent := range agents {
		created := callAppWS(t, conn, "tabs.create", map[string]any{
			"id": agent.tab, "workspaceId": layout.DefaultWorkspaceID, "position": i,
			"layout": "row", "firstPane": map[string]any{
				"id": agent.pane, "cwd": agent.cwd, "kind": "local", "sizeShare": 1,
			},
		}, 10+i)
		if created.Error != nil {
			t.Fatalf("create %s pane and tab: %+v", agent.name, created.Error)
		}
		opened := callAppWS(t, conn, "open", map[string]any{"paneId": agent.pane, "cols": 80, "rows": 24}, 20+i)
		if opened.Error != nil {
			t.Fatalf("open %s pane: %+v", agent.name, opened.Error)
		}
		var openedSession struct {
			SessionID string `json:"sessionId"`
		}
		if sessionDecodeErr := json.Unmarshal(opened.Result, &openedSession); sessionDecodeErr != nil || openedSession.SessionID == "" {
			t.Fatalf("decode %s session: id=%q err=%v", agent.name, openedSession.SessionID, sessionDecodeErr)
		}
		priorSessions[agent.pane] = session.ID(openedSession.SessionID)
	}
	// Approve the real shell enrolment path for each fake executable. This
	// records ordinary user consent before restart so the restored launch can
	// enrol without a dialog the app starts before a renderer can answer.
	approved := make(map[string]bool)
	for _, agent := range agents {
		if approved[agent.name] {
			continue
		}
		pending := pendingOf(t, first.agentEnroller.approval.Approve(ctx, priorSessions[agent.pane], agent.name))
		resolveAgentConsent(t, conn)
		if reason := settledOf(t, pending); reason != "" {
			t.Fatalf("initial %s consent settled with %q", agent.name, reason)
		}
		approved[agent.name] = true
	}
	_ = conn.Close()

	// The first four saved panes are real Git worktrees and use the location-
	// selected resume form. The next four are in one shared checkout and carry
	// distinct stable IDs. The last is a lazy-ID worker in that shared checkout.
	restarts := workers.NewFileRestartStore(storage.NewDocumentStore(paths.ConfigDir()), "worker-restarts.json")
	for _, agent := range agents {
		record := workers.RestartRecord{
			Participant: workers.ParticipantID("worker-" + agent.name + "-" + agent.tab),
			Group:       "acceptance", CoordinatorSession: "old-coordinator",
			PaneID: agent.pane, TabID: agent.tab, Agent: agent.name, Command: agent.name,
			Cwd:    agent.cwd,
			Resume: workers.ResumeIdentity{Mode: agent.mode, ID: agent.id},
		}
		if agent.hasWorktree {
			record.Worktree = workers.Worktree{Path: agent.cwd, Branch: agent.branch}
		}
		if recordErr := restarts.Record(ctx, record); recordErr != nil {
			t.Fatalf("persist %s restart identity: %v", agent.name, recordErr)
		}
	}
	first.Shutdown(ctx)
	endTheDaemon(t, helperBinary)

	restarted, restartedAppErr := newTestApp(t, withLocalHelperArtifacts(artifacts))
	if restartedAppErr != nil {
		t.Fatalf("New restarted app: %v", restartedAppErr)
	}
	if restartedStartErr := restarted.Start(ctx); restartedStartErr != nil {
		t.Fatalf("Start restarted app: %v", restartedStartErr)
	}
	t.Cleanup(func() { restarted.Shutdown(context.Background()) })
	conn = dialAppWS(t, restarted)
	defer func() { _ = conn.Close() }()
	visibleTabs := tabsInWindow(t, conn, 100)
	if len(visibleTabs) != len(agents) {
		t.Fatalf("tabs after restart = %v, want all %d worker tabs", visibleTabs, len(agents))
	}
	for _, agent := range agents {
		if !containsRestartTab(visibleTabs, agent.tab) {
			t.Errorf("saved %s tab %q is absent after restart; tabs=%v", agent.name, agent.tab, visibleTabs)
		}
	}

	for _, agent := range agents[:8] {
		logPath := filepath.Join(invocations, agent.name+".log")
		resumedSession, found := sessionForPane(restarted, agent.pane)
		if !found {
			t.Fatalf("%s pane %q has no session after restart", agent.name, agent.pane)
		}
		waittest.WaitForTimeoutDetail(t, agent.name+" resume invocation or explicit restore refusal", 15*time.Second, func() string {
			raw, readErr := os.ReadFile(logPath) //nolint:gosec // test-owned temporary path
			if readErr != nil {
				return fmt.Sprintf("invocation log unavailable: %v", readErr)
			}
			return fmt.Sprintf("invocation log contents: %q", string(raw))
		}, func() bool {
			if hasRestartInvocation(logPath, agent.cwd, agent.args) {
				return true
			}
			frame, frameErr := restarted.paneViews.Frame(resumedSession)
			if frameErr != nil {
				return false
			}
			for row := 0; row < frame.Rows; row++ {
				if strings.Contains(frame.Text(row), "could not resume") {
					return true
				}
			}
			return false
		})
		if !hasRestartInvocation(logPath, agent.cwd, agent.args) {
			frame, frameErr := restarted.paneViews.Frame(resumedSession)
			var reason []string
			if frameErr == nil {
				for row := 0; row < frame.Rows; row++ {
					reason = append(reason, frame.Text(row))
				}
			}
			t.Fatalf("%s did not launch its recorded %s resume in %q; restored pane says %q", agent.name, agent.mode, agent.cwd, strings.Join(reason, "\n"))
		}
		if resumedSession == string(priorSessions[agent.pane]) {
			t.Fatalf("%s pane session after restart = %q, want a new incarnation", agent.name, resumedSession)
		}
		waittest.WaitForTimeout(t, agent.name+" authenticated pane enrolment", 10*time.Second, func() bool {
			return restarted.paneViews.Watched(resumedSession)
		})
		if !restarted.paneViews.Watched(resumedSession) {
			t.Errorf("%s resumed session %q never enrolled in its saved pane", agent.name, resumedSession)
		}
	}

	// The lazy-ID record stays in the saved layout and never invokes Claude;
	// the browser acceptance observes its visible refusal at the user surface.
	claudeLog, err := os.ReadFile(filepath.Join(invocations, "claude.log")) //nolint:gosec // test-owned
	if err != nil {
		t.Fatalf("read Claude invocation log: %v", err)
	}
	if lines := strings.Split(strings.TrimSpace(string(claudeLog)), "\n"); len(lines) != 2 {
		t.Fatalf("Claude fake ran %d times, want worktree + stable-ID resumes and no lazy-ID guess: %q", len(lines), claudeLog)
	}
}

func writeRestartFakeCLI(t *testing.T, path, logDir string) {
	t.Helper()
	// The line is built first and appended with ONE printf: two panes of one
	// agent resume concurrently and share <agent>.log, and appending it piece
	// by piece interleaved their invocations into lines that match neither.
	tab := "\t"
	script := "#!/bin/sh\nlog=\"$NOCX_RESTART_LOG_DIR/${0##*/}.log\"\nline=\"$PWD\"\nfor arg do line=\"$line" + tab + "$arg\"; done\nprintf '%s\\n' \"$line\" >> \"$log\"\nexec cat >/dev/null\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // test executable
		t.Fatalf("write fake CLI %s: %v", path, err)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("create fake CLI log directory: %v", err)
	}
}

func hasRestartInvocation(logPath, cwd string, wantArgs []string) bool {
	raw, err := os.ReadFile(logPath) //nolint:gosec // test-owned temporary path
	if err != nil {
		return false
	}
	wantCWD, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < len(wantArgs)+1 {
			continue
		}
		gotCWD, resolveErr := filepath.EvalSymlinks(fields[0])
		if resolveErr != nil || gotCWD != wantCWD {
			continue
		}
		if reflect.DeepEqual(fields[1:1+len(wantArgs)], wantArgs) {
			return true
		}
	}
	return false
}

func TestHasRestartInvocationMatchesTheSameDirectoryThroughASymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "checkout")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "checkout-alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "invocation.log")
	wantArgs := []string{"claude", "--resume", "resume-id"}
	line := strings.Join(append([]string{target}, wantArgs...), "\t") + "\n"
	if err := os.WriteFile(logPath, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasRestartInvocation(logPath, alias, wantArgs) {
		t.Fatalf("invocation in %q was not matched to its symlinked spelling %q", target, alias)
	}
}

// Two panes of one agent resume at once after a restart, and both fake CLIs
// append to the same <agent>.log. Each invocation must land as one whole line,
// or the acceptance above cannot find an invocation that did happen.
func TestRestartFakeCLIRecordsConcurrentInvocationsAsWholeLines(t *testing.T) {
	root := t.TempDir()
	fake := filepath.Join(root, "bin", "codex")
	if err := os.MkdirAll(filepath.Dir(fake), 0o700); err != nil {
		t.Fatal(err)
	}
	logDir := filepath.Join(root, "log")
	writeRestartFakeCLI(t, fake, logDir)

	const n = 32
	cmds := make([]*exec.Cmd, n)
	for i := range cmds {
		cwd := filepath.Join(root, fmt.Sprintf("pane-%02d", i))
		if err := os.Mkdir(cwd, 0o700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(fake, "resume", fmt.Sprintf("session-%02d", i), "--flag") //nolint:gosec // test-owned fake CLI
		cmd.Dir = cwd
		cmd.Env = append(os.Environ(), "NOCX_RESTART_LOG_DIR="+logDir)
		cmds[i] = cmd
	}
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}

	logPath := filepath.Join(logDir, "codex.log")
	for i := range n {
		cwd := filepath.Join(root, fmt.Sprintf("pane-%02d", i))
		if !hasRestartInvocation(logPath, cwd, []string{"resume", fmt.Sprintf("session-%02d", i), "--flag"}) {
			raw, _ := os.ReadFile(logPath) //nolint:gosec // test-owned temporary path
			t.Fatalf("invocation %d is not one whole line in the shared log:\n%s", i, raw)
		}
	}
}

func createRestartAcceptanceWorktrees(t *testing.T) []string {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "source")
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatalf("create fixture repository: %v", err)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...) //nolint:gosec // test-owned fixture arguments
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.name", "nocx test")
	runGit("config", "user.email", "nocx-test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("test checkout\n"), 0o600); err != nil {
		t.Fatalf("write fixture commit: %v", err)
	}
	runGit("add", "README")
	runGit("commit", "-qm", "fixture")
	paths := make([]string, 4)
	for i, name := range []string{"claude", "codex", "omp", "prime-agent"} {
		paths[i] = filepath.Join(root, "worktree-"+name)
		branch := "restart-acceptance-" + strings.ReplaceAll(name, "-", "-")
		cmd := exec.Command("git", "-C", repo, "worktree", "add", "-q", "-b", branch, paths[i], "HEAD") //nolint:gosec // test-owned fixture paths
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create %s worktree: %v\n%s", name, err, output)
		}
	}
	return append(paths, repo)
}

func containsRestartTab(tabs []string, want string) bool {
	for _, tab := range tabs {
		if tab == want {
			return true
		}
	}
	return false
}
