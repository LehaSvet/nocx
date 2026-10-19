package agentdriver_test

// Observation.InputText, and the defect it replaces (nocx-6q1uh.18).
//
// internal/app/pane_messages.go used to answer "is the box empty" and "did
// the paste echo" by trimming strings.TrimSpace over regionText's raw
// cell-join of the whole minted target span, which for claude is
// Document.InputBox — the box's two rule rows included, by design (a menu
// displacing the box must still make a target minted from it refuse). A row
// of nothing but the rule's own full-width "─" is never whitespace, so that
// trim was never empty on any real frame: session.message could neither
// paste into an idle box (refused "someone is typing") nor confirm its own
// submission. See claude.go's own note for the corpus evidence and the fix.
//
// This file asserts the RULE's own answer instead, replayed off the same
// corpus claude.go's note cites.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/shady2k/nocx/internal/agentcapture"
	"github.com/shady2k/nocx/internal/agentdriver"
	"github.com/shady2k/nocx/internal/paneview"
)

// TestInputTextIsEmptyAtAnIdleClaudePrompt is the failure path's own
// baseline: a real idle input box, where the OLD trim-the-whole-span check
// was never empty because the span's own closing rule row is not
// whitespace. The rule's own inputText reading must say "empty" here, or
// nothing this bead fixed can ever paste.
func TestInputTextIsEmptyAtAnIdleClaudePrompt(t *testing.T) {
	reg := registry(t)
	for _, m := range []struct {
		capture string
		atMs    int64
	}{
		{"claude-2.1.266-turn", 36000},    // idle-120, the corpus's own free_text baseline
		{"claude-2.1.266-idle-60", 38000}, // idle-60, the narrow geometry
		{"claude-2.1.266-idle-80", 38000}, // idle-80
		{"claude-2.1.266-turn", 47500},    // working: the box the turn just cleared
	} {
		f := replay(t, m.capture, m.atMs)
		o := reg.Observe("claude", f)
		text, ok := o.InputText()
		if !ok {
			t.Errorf("%s@%dms (state=%s): InputText ok=false, want true (the rule reads this box)", m.capture, m.atMs, o.State)
			continue
		}
		if text != "" {
			t.Errorf("%s@%dms (state=%s): InputText = %q, want \"\" — the OLD trim-the-whole-span check answered non-empty here on every real frame, because the span's own closing rule row is not whitespace", m.capture, m.atMs, o.State, text)
		}
	}
}

// TestInputTextReadsWhatWasTyped is the success path a caller pastes on: the
// text a person (or session.message) actually typed, read off the box's own
// content rows with the prompt marker and any wrap indent stripped —
// including the two-row wrap the corpus actually shows, which is why the
// extractor reads more than the prompt row alone.
func TestInputTextReadsWhatWasTyped(t *testing.T) {
	reg := registry(t)
	cases := []struct {
		name    string
		capture string
		atMs    int64
		want    string
	}{
		{
			// Single content row: claude-2.1.266-turn at 47s, replayed with
			// `go run ./cmd/agent-capture replay -at 47000` while
			// investigating this bead — the box shows exactly
			// "❯ Write a 400 word story about a lighthouse keeper."
			// (a NO-BREAK SPACE after the marker, not the ordinary space it
			// prints as; see claude.go's own note).
			name:    "single line",
			capture: "claude-2.1.266-turn",
			atMs:    47000,
			want:    "Write a 400 word story about a lighthouse keeper.",
		},
		{
			// Two content rows: a typed line long enough to wrap at the
			// pane's own width. The continuation row's own two-cell indent
			// (ordinary spaces, not the marker's NO-BREAK SPACE) is what the
			// second half of the pattern strips, and the break's own space is
			// what the rule's "space" join puts back — the text here IS the
			// script's own line (lmstudio-subagent-finished.script), so a
			// reading that answered anything else would be a reading of a
			// text nobody typed. It answered "this\nfolder" until
			// nocx-xn63t.4.5.
			name:    "wrapped across two rows",
			capture: "claude-2.1.266-subagent-finished",
			atMs:    38000,
			want:    "Use the Agent tool to launch the Explore subagent with run_in_background true, asking it to list the files in this folder. Do not wait for it.",
		},
		{
			// The same wrap at a narrower geometry (60 columns), so the join
			// is asserted on more than one recording — and its own script line
			// (permission.script) is what it has to come back as.
			name:    "wrapped at 60 columns",
			capture: "claude-permission-60",
			atMs:    13000,
			want:    "Create a file named note.txt whose only content is the word hi",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := replay(t, c.capture, c.atMs)
			o := reg.Observe("claude", f)
			if o.State != agentdriver.StateFreeText {
				t.Fatalf("%s@%dms: state = %q, want free_text (the box this test reads must actually be idle-with-text)", c.capture, c.atMs, o.State)
			}
			text, ok := o.InputText()
			if !ok {
				t.Fatalf("%s@%dms: InputText ok=false, want true", c.capture, c.atMs)
			}
			if text != c.want {
				t.Errorf("%s@%dms: InputText = %q, want %q", c.capture, c.atMs, text, c.want)
			}
		})
	}
}

// wrappedEchoPasted is the one-paragraph task
// testdata/captures/scripts/session-message-wrapped-echo.script attaches to
// Claude's stdin as a BRACKETED PASTE (ESC[200~ … ESC[201~) — the exact bytes
// session.message's own paste step sends, since the runtime encodes a text
// atom with ghostty's paste encoder and Claude enables bracketed paste. It is
// the text the box under test has in it, and the text a reading of that box
// has to come back with.
const wrappedEchoPasted = "Please read internal/app/pane_messages.go and then explain, in a short paragraph, how a queued message longer than one row of the input box is pasted, how its echo is confirmed on the frame, and how the Enter key is finally sent by the coordinator that owns the queue. Finish by naming the file and the function where that confirmation happens. Do not change any file."

// wrappedEchoCapture is the recording of that paste, taken 2026-09-17 against
// Claude Code 2.1.272 on this machine through
// .claude/skills/nocx-detection-verify/record.sh (dead API endpoint — the
// paste starts no turn, so nothing here depends on a model answering). It is
// committed for this bead because no capture in the corpus had a box taller
// than the two rows an older Claude's wrap filled.
const wrappedEchoCapture = "claude-2.1.272-wrapped-echo"

// TestInputTextReadsASingleParagraphAcrossEveryWrappedRow is the reading
// session.message confirms its own paste on, on the shape the owner hit
// (nocx-xn63t.4.5): one paragraph pasted into the box, drawn over FOUR
// content rows because the box's own height grows with what is in it.
//
// The extractor this exercises used to read two rows down from the box's top
// rule — a cap sized when two rows was the tallest wrap in the corpus — so
// rows three and four were never read at all and the reading was a PREFIX of
// the pasted text. Nothing downstream could ever match it, which is why the
// delivery stopped at phase "partial" with the first two rows in boxContents
// and Enter was never pressed. The rows are read to the box's own closing
// rule now, and rejoined as Claude drew them: its wrap consumes the
// word-separating space at each row break (the paint bytes for this very
// frame go "\x1b[113Gthan\r\x1b[2C\x1b[1Bone" — no space is written anywhere
// between "than" and "one"), so a reading that merely dropped the wrap indent
// would answer "…longer thanone row…" and confirm nothing either.
func TestInputTextReadsASingleParagraphAcrossEveryWrappedRow(t *testing.T) {
	f := replay(t, wrappedEchoCapture, 50000)
	o := registry(t).Observe("claude", f)
	if o.State != agentdriver.StateFreeText {
		t.Fatalf("%s@50s: state = %q, want free_text (the box under test must be the live one)", wrappedEchoCapture, o.State)
	}
	// The premise, checked so this test cannot quietly stop covering the
	// shape it exists for: three or more content rows between the box's two
	// rules, which is one more than the cap the reading used to carry.
	if content := o.InputBox.Last - o.InputBox.First - 1; content < 3 {
		t.Fatalf("%s@50s: the box holds %d content rows (%+v), want 3 or more — this frame no longer exercises a wrap past the old cap", wrappedEchoCapture, content, o.InputBox)
	}
	text, ok := o.InputText()
	if !ok {
		t.Fatalf("%s@50s: InputText ok=false, want true — the rule reads this box", wrappedEchoCapture)
	}
	if text != wrappedEchoPasted {
		t.Errorf("%s@50s: InputText = %q,\nwant the text that was pasted       %q", wrappedEchoCapture, text, wrappedEchoPasted)
	}
}

// collapsedPasteCapture is the recording of a REALISTIC paste — 818 bytes,
// one paragraph, no newline in it, which is workers.Preamble's own shape — on
// Claude Code 2.1.296, taken 2026-10-18 through
// .claude/skills/nocx-detection-verify/record.sh against a dead endpoint with
// no Enter after the paste (nocx-xn63t.6.50; the README's own table has the
// five measured paste shapes side by side).
const collapsedPasteCapture = "claude-2.1.296-collapsed-paste"

// TestInputTextReadsClaudesCollapsedPaste is the reading a delivery confirms
// its own paste on when Claude does not draw the paste at all.
//
// Above a length Claude stops drawing what was pasted and draws a placeholder
// instead: `❯␣[Pasted text #1]` for a long single-paragraph paste (this
// capture — 818 bytes, no newline), `❯␣[Pasted text #1 +M lines]` for one that
// carries newlines (measured at M=9 and M=60, the README's table). The rule
// reads BOTH — the collapse row is a content row like any other, its text is
// what the box holds, and BoxEmpty must say "not empty" so nothing pastes a
// second message on top of it.
//
// This test is the reason the defect it was written for is NOT in the rule:
// session.message's echo check answered "the box does not show this text" on
// this frame, while the rule had read the box correctly all along.
func TestInputTextReadsClaudesCollapsedPaste(t *testing.T) {
	reg := registry(t)
	f := replay(t, collapsedPasteCapture, 45000)
	o := reg.Observe("claude", f)
	if o.State != agentdriver.StateFreeText {
		t.Fatalf("%s@45s: state = %q, want free_text — a box holding a paste is the live prompt", collapsedPasteCapture, o.State)
	}
	text, ok := o.InputText()
	if !ok {
		t.Fatalf("%s@45s: InputText ok=false, want true — the rule reads this box", collapsedPasteCapture)
	}
	const want = "[Pasted text #1]"
	if text != want {
		t.Errorf("%s@45s: InputText = %q, want %q — Claude's placeholder, with no line count in it, is what a long one-paragraph paste becomes", collapsedPasteCapture, text, want)
	}
	if o.BoxEmpty() {
		t.Errorf("%s@45s: BoxEmpty = true, want false — the box holds the placeholder of a paste that is waiting to be submitted, so a second message must not paste on top of it", collapsedPasteCapture)
	}
	// The premise, checked so this test cannot quietly stop covering the
	// collapse it exists for: the pasted text itself is NOWHERE on screen.
	// Claude drew a placeholder, which is the whole reason the echo check
	// could not match the text it had just pasted.
	for y := range f.Lines {
		if strings.Contains(f.Text(y), "You are a worker, and your coordinator is") {
			t.Fatalf("%s@45s: row %d still carries the pasted text — this frame no longer exercises Claude's collapse", collapsedPasteCapture, y)
		}
	}
}

// ── BoxEmpty: the ONE statement both writers ask ─────────────────────────

// BoxEmpty fixes what "the box is empty" means for BOTH writers that need it
// (nocx-xn63t.6.44): internal/app's session.message before it pastes a queued
// message, and internal/agenttyping's grant before a wake is typed into a
// coordinator's pane. Two spellings would agree on every frame anybody tried
// and disagree on the one nobody did, so the predicate is asserted HERE and
// not only through its callers.
//
// The middle cases are the ones a shorter predicate gets wrong, and they fail
// in opposite directions. Claude's own faint suggestion is drawn inside the
// box and is NOT text in it (reading the paint as text refuses every delivery
// while the prompt stands, nocx-xn63t.6.32). A menu that has displaced the box
// is a box NOBODY READ — not an empty one — and admitting a write on it is
// exactly the concatenation this predicate exists to prevent.
func TestBoxEmptyIsTheOnlyStatementOfWhatAnEmptyBoxIs(t *testing.T) {
	reg := registry(t)
	cases := []struct {
		name    string
		agent   string
		capture string
		atMs    int64
		read    bool // InputText's ok: the rule read a box on this exact frame
		want    bool
	}{
		{"an idle prompt", "claude", "claude-2.1.266-turn", 36000, true, true},
		{"claude's own suggestion in the box", "claude", "claude-idle", 1470, true, true},
		{"the person's own draft", "claude", "claude-2.1.266-turn", 47000, true, false},
		{"a menu has displaced the box", "claude", "claude-2.1.266-permission", 49000, false, false},
		{"an agent this build has no rule for", "no-such-agent", "claude-2.1.266-turn", 36000, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := reg.Observe(c.agent, replay(t, c.capture, c.atMs))
			_, ok := o.InputText()
			if ok != c.read {
				read := "not read"
				if c.read {
					read = "read"
				}
				t.Fatalf("%s@%dms: InputText ok = %v, want %v — this case is written for a box that is %s",
					c.capture, c.atMs, ok, c.read, read)
			}
			if got := o.BoxEmpty(); got != c.want {
				t.Fatalf("%s@%dms (state %q, InputText read=%v): BoxEmpty = %v, want %v",
					c.capture, c.atMs, o.State, ok, got, c.want)
			}
		})
	}
}

// TestInputTextIsUnreadableWhenAMenuDisplacesTheBox is the paired failure
// this projection must keep: a menu moment has no box to read at all
// (nocx-6q1uh.10, menuDisplacesInputBox), and InputText must say so with
// ok==false rather than text=="" — the two are different claims, and only
// "the rule read this box and it's empty" is a licence to paste.
func TestInputTextIsUnreadableWhenAMenuDisplacesTheBox(t *testing.T) {
	reg := registry(t)
	f := replay(t, "claude-2.1.266-permission", 49000) // bash-permission
	o := reg.Observe("claude", f)
	if o.State != agentdriver.StatePermissionChoice {
		t.Fatalf("state = %q, want permission_choice", o.State)
	}
	if text, ok := o.InputText(); ok {
		t.Fatalf("InputText = (%q, true), want ok=false — a menu has displaced the box, there is nothing to read", text)
	}
}

// TestInputTextIsEmptyWhenClaudeDrawsItsOwnSuggestion is the failure path
// nocx-xn63t.6.32 measured on a real session: Claude's empty prompt draws a
// SUGGESTION in the box it has not been given, and that suggestion is not
// text the person typed.
//
// Observed 2026-10-17 on Claude Code 2.1.296 (owner's working day): the
// coordinator answered a worker pane's folder-trust question, and four
// session.read calls over the following minute all reported the same free_text
// pane whose box row read "\u276f\u00a0Try \"fix lint errors\"" while the worker's queued
// task stayed at phase "queued" — no delivery, no refusal, no wait. The
// reading below is why: the suggestion is drawn inside the box's own content
// rows, so the inputText extractor captured it as the box's text, and every
// caller that gates on "the box is empty" answered "someone is typing" for a
// box nobody had touched. The pane's classification never changes while the
// suggestion sits there, so nothing wakes the queue either; the safety recheck
// re-reads the same non-empty reading forever.
//
// The marks are the corpus's own recorded frames carrying that drawing —
// claude-modal and claude-idle, replayed at the milliseconds the suggestion is
// on screen (measured, not guessed: the ghost is present from 1473ms to about
// 1530ms in claude-modal and near 1450-1500ms in claude-idle, and nowhere else
// in either capture). The suggestion is drawn in the terminal as SGR 2 (faint)
// — the paint bytes are "\x1b[39m\u276f\u00a0\x1b[2mTry \"fix lint errors\"" — which is
// the rendering that distinguishes it from typed text and the one thing this
// frame type does not carry (paneview.Cell is Text and Width by design), so
// the rule has to know Claude's own form.
//
// The paired direction is the rest of this file: TestInputTextReadsWhatWasTyped
// asserts that content a person actually typed is still read, so a rule that
// answered "" for every prompt row could not pass both.
func TestInputTextIsEmptyWhenClaudeDrawsItsOwnSuggestion(t *testing.T) {
	reg := registry(t)
	for _, m := range []struct {
		capture string
		atMs    int64
		drawn   string
	}{
		{"claude-modal", 1480, `Try "fix lint errors"`},
		{"claude-idle", 1470, `Try "fix typecheck errors"`},
	} {
		f := replay(t, m.capture, m.atMs)
		o := reg.Observe("claude", f)
		if o.State != agentdriver.StateFreeText {
			t.Fatalf("%s@%dms: state = %q, want free_text — a fresh prompt with a suggestion on it is an idle pane", m.capture, m.atMs, o.State)
		}
		// The premise, checked so this test cannot quietly stop covering the
		// drawing it exists for: the frame really does carry the suggestion,
		// on the one row the box's own grammar reads as content.
		onScreen := false
		for y := range f.Lines {
			if strings.Contains(f.Text(y), m.drawn) {
				onScreen = true
				break
			}
		}
		if !onScreen {
			t.Fatalf("%s@%dms: no row carries %q — this frame no longer exercises the drawing", m.capture, m.atMs, m.drawn)
		}
		text, ok := o.InputText()
		if !ok {
			t.Fatalf("%s@%dms: InputText ok=false, want true (the rule reads this box)", m.capture, m.atMs)
		}
		if text != "" {
			t.Errorf("%s@%dms: InputText = %q, want \"\" — Claude's own suggestion is not text in the box, and reading it as text leaves a caller that gates on an empty box refusing every delivery for as long as the prompt stands (nocx-xn63t.6.32)", m.capture, m.atMs, text)
		}
	}
}

// ── BY ATTRIBUTE, NOT BY WORDS (nocx-xn63t.6.56) ──────────────────────────

// THE SUGGESTION IS READ OFF THE RENDERING IT WAS DRAWN WITH, and this test
// states both directions on ONE recording and ONE set of characters: the row
// Claude paints its suggestion into, repainted with the same characters solid.
//
// Measured before the frame carried the attribute (nocx-xn63t.6.52, on this
// corpus): the recorded SGR 2 drawing and the same characters painted WITHOUT
// SGR 2 both read BoxEmpty true, so a person typing `Try "…"` was read as an
// empty box and had a wake pasted into their sentence. The rule could only tell
// the two apart by the WORDS, which is a spelling Claude is free to change and
// a sentence a person is free to type.
func TestThePromptSuggestionIsReadByItsAttributeAndNotByItsWords(t *testing.T) {
	reg := registry(t)
	for _, m := range []struct {
		capture string
		atMs    int64
		drawn   string
	}{
		{"claude-modal", 1480, `Try "fix lint errors"`},
		{"claude-idle", 1470, `Try "fix typecheck errors"`},
	} {
		t.Run(m.capture, func(t *testing.T) {
			r := newReplayer(t, m.capture, m.atMs)
			recorded, err := r.Frame()
			if err != nil {
				t.Fatalf("frame: %v", err)
			}

			// The premise, checked so this test cannot quietly stop covering
			// the rendering it exists for: on the RECORDED frame the cells the
			// suggestion occupies are faint and the marker beside them is not.
			cells := promptRowCells(t, recorded, m.drawn)
			if cells[0].Text != promptMarker {
				t.Fatalf("%s@%dms: the prompt row opens with %q, want the marker %q", m.capture, m.atMs, cells[0].Text, promptMarker)
			}
			if cells[0].Faint {
				t.Fatalf("%s@%dms: the prompt marker is drawn faint, so this frame no longer separates chrome from the drawing", m.capture, m.atMs)
			}
			var content strings.Builder
			typed, faintContent := false, true
			for _, c := range cells[1:] {
				// The marker's own separator is chrome: Claude draws ONE
				// NO-BREAK SPACE cell between "❯" and the box's content, and
				// the rule's pattern strips it (claude.go's own note).
				if c.Text == "" || c.Text == "\u00a0" {
					continue
				}
				content.WriteString(c.Text)
				if strings.TrimSpace(c.Text) == "" {
					continue
				}
				typed = true
				if !c.Faint {
					faintContent = false
				}
			}
			if got := strings.TrimRight(content.String(), " "); got != m.drawn {
				t.Fatalf("%s@%dms: the row after the marker reads %q, want %q", m.capture, m.atMs, got, m.drawn)
			}
			if !typed || !faintContent {
				t.Fatalf("%s@%dms: the recorded suggestion is not wholly faint (content=%v, all faint=%v), so this test no longer exercises the attribute",
					m.capture, m.atMs, typed, faintContent)
			}

			// The recorded drawing is the box EMPTY: nothing has been given to
			// this pane, and every caller that gates on "the box is empty" must
			// paste rather than wait.
			o := reg.Observe("claude", recorded)
			if o.State != agentdriver.StateFreeText {
				t.Fatalf("%s@%dms: state = %q, want free_text", m.capture, m.atMs, o.State)
			}
			if text, ok := o.InputText(); !ok || text != "" {
				t.Fatalf("%s@%dms: InputText = (%q, %v), want (\"\", true)", m.capture, m.atMs, text, ok)
			}
			if !o.BoxEmpty() {
				t.Errorf("%s@%dms: BoxEmpty = false, want true — Claude's suggestion is drawn into an empty box", m.capture, m.atMs)
			}

			// The SAME characters, painted WITHOUT SGR 2, on the same recording
			// and the same row. This is a person typing that line: it is text in
			// the box and nothing may be pasted into it.
			solid := paintPromptRow(t, r, recorded, m.drawn, false)
			so := reg.Observe("claude", solid)
			if so.State != agentdriver.StateFreeText {
				t.Fatalf("%s@%dms: state after the solid repaint = %q, want free_text", m.capture, m.atMs, so.State)
			}
			text, ok := so.InputText()
			if !ok {
				t.Fatalf("%s@%dms: InputText ok=false after the solid repaint, want true", m.capture, m.atMs)
			}
			if text != m.drawn {
				t.Errorf("%s@%dms: the same characters painted solid read %q, want %q — a person typing that line is not an empty box, and reading them alike is the whole defect (nocx-xn63t.6.56)",
					m.capture, m.atMs, text, m.drawn)
			}
			if so.BoxEmpty() {
				t.Errorf("%s@%dms: BoxEmpty = true with %q typed into the box", m.capture, m.atMs, m.drawn)
			}

			// And painted FAINT again, the same characters read empty once more.
			faint := paintPromptRow(t, r, solid, m.drawn, true)
			fo := reg.Observe("claude", faint)
			if text, ok := fo.InputText(); !ok || text != "" {
				t.Errorf("%s@%dms: the same characters painted faint read (%q, %v), want (\"\", true)", m.capture, m.atMs, text, ok)
			}
			if !fo.BoxEmpty() {
				t.Errorf("%s@%dms: BoxEmpty = false for the faint drawing", m.capture, m.atMs)
			}
		})
	}
}

// paintPromptRow repaints the row of f that carries drawn with the SAME
// characters, drawn faint (SGR 2) or solid, and answers the screen afterwards.
// Nothing but the rendering differs between the two directions — same
// recording, same row, same bytes of text.
func paintPromptRow(t *testing.T, r *replayer, f paneview.Frame, drawn string, faint bool) paneview.Frame {
	t.Helper()
	row := -1
	for y := range f.Lines {
		if strings.Contains(f.Text(y), drawn) {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("no row of this frame carries %q", drawn)
	}
	paint := "\x1b[" + strconv.Itoa(row+1) + ";1H" + promptMarker + "\u00a0"
	if faint {
		paint += "\x1b[2m"
	}
	paint += drawn + "\x1b[0m"
	if err := r.Feed([]agentcapture.Chunk{{AtMs: r.mark, Data: []byte(paint)}}); err != nil {
		t.Fatalf("paint: %v", err)
	}
	painted, err := r.Frame()
	if err != nil {
		t.Fatalf("frame after the repaint: %v", err)
	}
	return painted
}

// promptMarker is the glyph Claude's input box opens with, at column 0. It is
// spelled once here so the repaint above and the frame it lands on agree.
const promptMarker = "\u276f"

// promptRowCells answers the non-blank cells of the row of f that draws drawn,
// left to right.
func promptRowCells(t *testing.T, f paneview.Frame, drawn string) []paneview.Cell {
	t.Helper()
	for y := range f.Lines {
		if !strings.Contains(f.Text(y), drawn) {
			continue
		}
		cells := make([]paneview.Cell, 0, len(f.Lines[y]))
		for _, c := range f.Lines[y] {
			if c.Text == "" {
				continue
			}
			cells = append(cells, c)
		}
		if len(cells) == 0 {
			t.Fatalf("row %d carries %q in its text and no cells", y, drawn)
		}
		return cells
	}
	t.Fatalf("no row of this frame carries %q", drawn)
	return nil
}
