package app

// What the preamble is ALLOWED to name (nocx-luqz9.5, acceptance 2).
//
// The text a worker is handed tells it which calls it has. A name in that text
// that the registry does not declare is a worker sent to a call that does not
// exist: it will make the call, be refused, and — since the refusal is the only
// thing it learns — report nothing. The failure is silent in exactly the way
// this epic exists to end, so it is not left to a reader's eye: every tool name
// in the text is looked up in the ASSEMBLED registry here, and separately in
// the set the worker's own grant OFFERS it.
//
// TWO CHECKS, AND BOTH ARE LOAD-BEARING. A declaration can exist and still not
// reach a worker — that is A11's whole construction (a worker's grant names a
// workspace sub-scope and no session), so a preamble naming a coordinator's
// tool would satisfy the first check and fail the worker it was written for.
// The registry's declarations are the source of truth for both; the text is
// scanned, never copied, so a preamble edited to name a new tool is checked the
// moment the edit lands.
//
// AND THE CHECK IS PROVED TO BITE, in this same file, rather than assumed to:
// the negative half of the table below runs the SAME scan over a real preamble
// with one tool name swapped for the two ways a name can be wrong — one nothing
// declares, and one that is declared and withheld from a worker. A scan that
// had quietly stopped matching would pass every positive case here and fail
// those two.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agenttools"
	"github.com/shady2k/nocx/internal/workers"
)

// preambleToolName is the shape a worker's own call takes in the text: the
// namespace and the tool, exactly as the wire names them, and NOT the sentence
// punctuation around them — a scanner greedy for dots would read the full stop
// at the end of a clause as part of the tool's name and fail on prose that is
// perfectly correct. It is a scanner and not a list of expected names: a list
// here would be the second copy of the vocabulary this test exists to prevent.
var preambleToolName = regexp.MustCompile(`workers\.[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)*`)

// preambleToolProblems names every tool the text sends a worker to that the
// registry does not declare, or declares without offering it to a worker.
//
// It answers rather than asserting so that the same check can be run over texts
// nobody ships — see the negative table below, which is what makes the positive
// case evidence instead of a claim.
func preambleToolProblems(t *testing.T, registry agenttools.Registry, text string) []string {
	t.Helper()
	names := preambleToolName.FindAllString(text, -1)
	if len(names) == 0 {
		// NOT AN EMPTY ANSWER: a text naming no tool at all is the one case
		// this scan cannot check, and saying so here is what stops a preamble
		// rewritten into prose from passing by having nothing to look up.
		return []string{"the text names no tool at all, so this check can say nothing about it"}
	}
	offered := make(map[string]bool)
	for _, tool := range registry.ForGrant(participantGrant(workerTestWorkspace)) {
		offered[tool.Name] = true
	}
	var problems []string
	for _, name := range names {
		if _, ok := registry.Lookup(name); !ok {
			problems = append(problems, name+" is named but no declaration in the registry carries it")
			continue
		}
		if !offered[name] {
			problems = append(problems, name+" is declared but this worker's own grant does not offer it")
		}
	}
	return problems
}

func TestEveryToolThePreambleNamesIsDeclaredAndOfferedToAWorker(t *testing.T) {
	registry, err := agenttools.Assemble(os.DirFS("../../contracts/tools"))
	if err != nil {
		t.Fatalf("assemble the real tool registry: %v", err)
	}
	text := workers.Preamble("sess-coordinator")
	for _, problem := range preambleToolProblems(t, registry, text) {
		t.Errorf("%s:\n%s", problem, text)
	}
}

// The negative half: the same scan, run over texts that ARE wrong, in each of
// the two ways a preamble can be. Each case is the REAL preamble with one name
// swapped, so what fails is the swap and not the prose around it.
func TestThePreambleToolScanRejectsANameAWorkerCannotCall(t *testing.T) {
	registry, err := agenttools.Assemble(os.DirFS("../../contracts/tools"))
	if err != nil {
		t.Fatalf("assemble the real tool registry: %v", err)
	}
	real := workers.Preamble("sess-coordinator")
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "a tool nothing declares",
			text: strings.Replace(real, "workers.report", "workers.declare", 1),
			want: "no declaration in the registry carries it",
		},
		{
			name: "a tool declared but withheld from a worker",
			text: strings.Replace(real, "workers.inbox", "workers.spawn", 1),
			want: "this worker's own grant does not offer it",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The premise, so a case cannot pass because the replacement did
			// not land: the text must differ from the real one.
			if tc.text == real {
				t.Fatalf("the mutation did not apply, so this case asserts nothing:\n%s", tc.text)
			}
			problems := preambleToolProblems(t, registry, tc.text)
			if len(problems) == 0 {
				t.Fatalf("the scan passed a preamble naming %s:\n%s", tc.name, tc.text)
			}
			joined := strings.Join(problems, "; ")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("problems = %q, want one naming %q", joined, tc.want)
			}
		})
	}
}

// The two calls the design names are named. This test is the floor under the
// scan above: a preamble rewritten to talk about the mailbox and the report in
// prose — still true, still readable, and no longer something a worker can act
// on — passes the "every named tool exists" check by naming none.
func TestThePreambleNamesTheTwoCallsAWorkerActuallyHas(t *testing.T) {
	text := workers.Preamble("sess-coordinator")
	for _, want := range []string{"workers.report", "workers.inbox"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the preamble never names %s:\n%s", want, text)
		}
	}
}

// Criterion (nocx-xn63t.6.46): every tool the preamble names carries the
// selector a DEFERRING harness needs to load it.
//
// A Claude Code session is not handed nocx's tools as native ones: the MCP
// tools of the nocx server are deferred, and a worker told only
// `workers.report` answered that "the tool is not exposed as a native tool"
// and called the socket by hand — the report this finding was filed from. The
// harness-visible name is `mcp__nocx__workers_report`, and the selector that
// loads it is `select:` in front of that name, so the expectation is DERIVED
// from the names the scan above finds in the text rather than written out as a
// second list: a tool added to the preamble tomorrow reds this test until its
// selector is written beside it.
//
// AND THE CHECK IS PROVED TO BITE, the way the scan above is, with a text that
// is wrong in each of the two ways it can be. A scan that had quietly stopped
// matching would pass every real preamble and fail nothing.
func TestThePreambleCarriesAToolSearchSelectorForEveryToolItNames(t *testing.T) {
	text := workers.Preamble("sess-coordinator")
	for _, problem := range preambleSelectorProblems(text) {
		t.Errorf("%s:\n%s", problem, text)
	}
}

// preambleSelectorProblems names every mismatch between the tools the text
// tells a worker about and the selectors it gives for loading them: a tool
// with no selector, a selector for a tool the text never names, and a text
// carrying none at all.
func preambleSelectorProblems(text string) []string {
	names := preambleToolName.FindAllString(text, -1)
	if len(names) == 0 {
		// Same reasoning as the scan above: a text naming no tool at all is the
		// one case this check cannot answer, and saying so is what stops a
		// preamble rewritten into prose from passing by having nothing to check.
		return []string{"the text names no tool at all, so this check can say nothing about it"}
	}
	var problems []string
	named := make(map[string]bool, len(names))
	for _, name := range names {
		selector := claudeToolSearchSelector(name)
		named[selector] = true
		if !strings.Contains(text, selector) {
			problems = append(problems, name+" is named but the text gives no "+selector+" to load it where tools are deferred")
		}
	}
	selectors := preambleSelector.FindAllString(text, -1)
	if len(selectors) == 0 {
		problems = append(problems, "the text gives no ToolSearch selector at all")
	}
	for _, selector := range selectors {
		if !named[selector] {
			problems = append(problems, selector+" loads nothing this text tells a worker about")
		}
	}
	return problems
}

// The negative half: the same check over the REAL preamble with one part
// removed or added, so what fails is the change and not the prose around it.
func TestThePreambleSelectorCheckRejectsAToolWithoutAWayToLoadIt(t *testing.T) {
	real := workers.Preamble("sess-coordinator")
	for _, tc := range []struct {
		name string
		text string
		want string
	}{
		{
			name: "a tool named with its selector taken away",
			text: strings.Replace(real, " or select:mcp__nocx__workers_inbox", "", 1),
			want: "workers.inbox is named but the text gives no select:mcp__nocx__workers_inbox",
		},
		{
			name: "a selector for a tool the text never names",
			text: real + " select:mcp__nocx__workers_spawn",
			want: "select:mcp__nocx__workers_spawn loads nothing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The premise, so a case cannot pass because the change did not land.
			if tc.text == real {
				t.Fatalf("the mutation did not apply, so this case asserts nothing:\n%s", tc.text)
			}
			problems := preambleSelectorProblems(tc.text)
			if len(problems) == 0 {
				t.Fatalf("the check passed a preamble with %s:\n%s", tc.name, tc.text)
			}
			joined := strings.Join(problems, "; ")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("problems = %q, want one naming %q", joined, tc.want)
			}
		})
	}
}

// preambleSelector is the shape of one ToolSearch selector in the text.
var preambleSelector = regexp.MustCompile(`select:mcp__nocx__[a-z][a-z0-9_]*`)

// claudeToolSearchSelector is the ToolSearch selector that loads one nocx tool
// in Claude Code: the server's own prefix and the tool's name, with the
// namespace separator the harness shows spelled as an underscore —
// `workers.report` is `mcp__nocx__workers_report`, and the selector is that
// name behind `select:`.
func claudeToolSearchSelector(tool string) string {
	return "select:mcp__nocx__" + strings.ReplaceAll(tool, ".", "_")
}
