// Package agenttyping is the typing primitive: nocx puts bytes into a pane it
// is watching, and only when that pane's own rule says it may (nocx-dkawo.1,
// D14 of the orchestration mechanism design).
//
// # What this exists to refuse
//
// A mistimed keystroke does not merely fail to arrive. It ANSWERS whatever
// modal is on screen, and the modal a coding agent puts up is a tool approval
// whose first option is Yes — so the failure is not a lost keystroke, it is a
// tool call the person never saw being approved on their behalf. An earlier
// draft proposed sending Escape first so a mistimed keystroke would be
// recoverable; that is strictly worse than refusing, because Escape ANSWERS the
// modal too. There is no mitigation here, only refusal.
//
// So: type only on a positively identified free_text whose input box is EMPTY.
// A permission menu, a user-opened menu, a spinner, the TUI's own error, an
// unknown and a pane nocx is not watching all receive NOTHING AT ALL, and the
// refusal is recorded with the state that refused. unknown is not a gap to be
// optimistic about: every consumer of internal/agentdriver treats it as busy,
// precisely so that the failure direction is a refusal.
//
// The empty box joins that list rather than sitting beside it (nocx-xn63t.6.44):
// a free_text pane with a draft in its box is a pane whose owner is MID-SENTENCE.
// A line written there is appended to what they typed and submitted with it —
// their question to their agent arrives with nocx's sentence in front of it —
// and the state gate alone cannot see the difference, because free_text is
// exactly what waiting-for-input looks like whether or not somebody has started
// writing. The predicate is agentdriver.Observation.BoxEmpty, the same one
// session.message pastes on, so the two callers cannot disagree about what an
// empty box is.
//
// # A line nocx could not submit is its own, and it does not block the next wake
//
// The box gate has one state it cannot treat as somebody else's draft, and that
// is the one nocx's own partial submission leaves (nocx-xn63t.6.53). The text
// segment is accepted and the submit key is then refused — the screen moved
// between their two frames — so the typist answers OutcomeTyped and its own line
// stays standing in the box. Read against the empty-box gate, every later wake
// is refused for as long as it stands, and unlike a Claude suggestion
// (nocx-xn63t.6.52) nothing ends it: there is no timeout, and the only actor who
// could clear that box is the one being refused.
//
// So the bytes are REMEMBERED and they stay nocx's: after a paste it could not
// submit, the pane's box holds exactly what this package wrote, and the next
// grant is admitted when — and only when — the rule reads back exactly those
// bytes. What is pressed then is the one key that was refused, the submit key,
// rather than a second line pasted on top of the first. That keeps the person's
// protection exactly as it was: a box holding anything else, their sentence or
// their sentence with nocx's line in front of it, is not ours and is refused as
// before. The record is dropped the moment the line leaves the box, so the same
// characters appearing later are a person's and not a licence.
//
// # The gate is the permit's constructor, not a row of ifs
//
// A caller of this package never reaches the pane's input queue. What reaches
// it is a permit, and grant is the only thing in this package that builds one:
//
//   - the pane's current state is free_text, POSITIVELY IDENTIFIED from a frame
//     read inside grant — never remembered from an earlier one, never supplied
//     by the caller;
//   - the agent's rule has EARNED typing authority (agentcalib.Verdict.MayType,
//     a value whose backing field is unexported in that package and written in
//     exactly one statement, so nothing here can manufacture it);
//   - the box the line would go into is empty, read off the frame that mints
//     the permit and again off the frame that authorises the paste — and
//     never off the submit key's, because by then the box holds the text this
//     call just pasted.
//
// Every early return in grant returns a refusal and a ZERO permit, and a zero
// permit's `by` is nil, so put writes nowhere. The pane and the agent live
// INSIDE the permit — the pattern agenttools.Runner uses for a session grant —
// so a holder of one cannot name another pane, and the agent is never a
// parameter: it comes from the enrolment act, exactly as a calibration's label
// comes from the step rather than from the surface.
//
// # THE INTERVAL, AND BOTH ITS ENDS
//
// It opens at the frame read whose classification is free_text and closes when
// the pane's input queue has accepted that segment's bytes. Between those two
// moments the screen can move, and a decision that is stale by the time it is
// acted on is exactly the mistimed keystroke — so the decision is RE-TAKEN from
// a frame read inside put, immediately before the write, with nothing between
// the two but the queue call itself. The bytes are built before the read, so
// the window holds no work at all.
//
// The permit's own grant is therefore not what authorises a write; it is what
// authorises an ATTEMPT. Each segment gets its own interval and its own frame,
// which is why the submit key — the one byte that can answer a modal — is
// gated on a frame read microseconds before it rather than on the frame that
// admitted the text.
//
// What BOUNDS the residual window is a property of the state itself, and it is
// worth stating because it is the reason this is shippable. A pane classified
// free_text has no turn in flight: that is what the classification means, and
// internal/agentdriver's rule sends a turn in flight, a background child, a
// menu, a dialog and any chrome it has not seen to other states. An agent with
// no turn in flight emits nothing, so in that window there is no producer that
// could raise a modal. The one producer that remains is the PERSON at that
// pane's keyboard, and their keystroke racing ours cannot be excluded by any
// local mechanism — their bytes and ours reach one pty from two goroutines.
// That residual is named rather than closed, and its cost is bounded: text
// interleaved in an input box, never an answer to a dialog, because a dialog
// cannot appear without a turn.
//
// # The submission is two writes, and never one
//
// The text is framed as a bracketed paste and the submit key is pressed
// SEPARATELY, so it cannot be swallowed as paste content — and so the byte that
// submits is gated on its own frame. nelix measured the paste framing at 0.0s
// against 2.2s for raw echo on 61.5 KB, so this is also the only form that
// scales.
//
// The framing assumes the receiving program enabled bracketed paste, which this
// package cannot ask a Frame about. It is not a hole: the only panes reachable
// here are panes whose agent's rule has been verified against a labelled set —
// a full-screen TUI — and if one of them did not enable it, the marker bytes
// land in an input box as literal text rather than as an answer to anything,
// because the gate has already established there is nothing on screen to
// answer.
//
// # A menu used to be answered here; it no longer is (design §9/§11, Task 11)
//
// This package once had a second door onto a pane's input queue, Choose,
// reached on a positively identified menu and writing only the closed set a
// menu offers: a key that moves the selection one row, and the key that
// confirms it — the same option-loop discipline (gate every key on a frame
// read inside the call, confirm only on a frame that shows the selection ON
// the named option) that answering a menu still requires. That discipline
// now lives in internal/app/session_keys.go's PaneKeys, under the SAME two
// gates (a bound target, a fresh frame per step) session.keys' text atom
// already spends, rather than a second implementation of it here. Submit and
// Type below are what this package still owns: writing text into a pane's
// input queue, for a coordinator's own wake (workers.go's workerWaker).
//
// # Delivery is UNACKNOWLEDGED
//
// Accepted is not delivered. The queue taking a job says the bytes are in line
// for the pty, and seeing our text echo in the input region would be evidence
// it was typed, never that it was acted on. Nothing here closes a fact; only
// the participant's own subsequent call does.
//
// # Local only
//
// The remote helper types on the far side (D14) and that is not this package.
// What is written here goes to a session's own input queue, in this process.
package agenttyping

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/shady2k/nocx/internal/agentcalib"
	"github.com/shady2k/nocx/internal/agentdriver"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/paneview"
	"github.com/shady2k/nocx/internal/sessionruntime"
)

// MaxText bounds one submission, in bytes. A wake carries a sentence and a
// dispatch carries a brief; 64 KiB is well past both and well under the point
// where one job could starve a session's bounded write queue.
const MaxText = 64 << 10

// The submission's bytes, spelled once. bracketed paste around the text so a
// newline inside it is content rather than a submission, and the submit key on
// its own so it cannot be swallowed as paste content.
const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
	submitKey  = "\r"
)

// emptyBoxRefusal is the sentence a box that is not empty refuses with, in the
// same words at both reads — the permit's and the paste's own frame — so a
// person is told what stopped the write and not which routine noticed it.
const emptyBoxRefusal = "nocx types into a pane whose input box it can read and is empty, and this one is neither"

// ownLineClause is what a refusal about nocx's own standing line adds: the bytes
// in that box are this package's, so a person reading the record knows the box is
// not holding their draft and that the next attempt will finish the submission
// rather than wait for them (nocx-xn63t.6.53).
const ownLineClause = " (the line standing in that box is the one nocx typed and could not submit)"

// ownLineMovedRefusal is the sentence a submission refuses with when the box it
// was admitted on no longer holds only nocx's own line — a keystroke landed
// between the grant and the key. It is a constant for the same reason
// emptyBoxRefusal is: the same situation must read the same way at every read,
// so a person reading the log sees one fact and not two.
const ownLineMovedRefusal = "the box no longer holds only the line nocx typed there, so no submit key was pressed"

// Screens is the seam onto a pane's live grid (AD-8). One method, because
// typing may READ a screen and may not enrol, withdraw, resize or classify one.
type Screens interface {
	Frame(paneID string) (paneview.Frame, error)
}

// Rules reads one frame under one agent's rule. The concrete registry
// satisfies it; the seam exists so this package depends on the abstraction and
// not on the registry's other powers.
//
// It is Observe and not Classify because a write needs TWO things from one
// frame — the state, and whether the box the person types in is empty — and
// asking twice would be two evaluations of one frame that could disagree
// (Registry.Classify is Observe's own scalar projection for exactly this
// reason). One call, one reading, both answers.
type Rules interface {
	Observe(agent string, f paneview.Frame) agentdriver.Observation
}

// Authority answers what an agent's rule has EARNED against the frames a person
// labelled for it (nocx-jse6x).
//
// The answer is a value this package cannot build: agentcalib.Verdict's backing
// field is unexported and written in one statement inside Verify, so every
// other way a Verdict can come about — a struct literal, a zero value, a failed
// lookup — denies typing without anybody having remembered to check.
type Authority interface {
	Verify(ctx context.Context, agent string) agentcalib.Verdict
}

// Enrolment answers which agent a pane was enrolled under.
//
// It is here so that the agent is never a parameter of a typing call. A caller
// that could name the agent could name one whose rule verifies while the pane
// runs something else, and the frame would then be read by a rule that was
// never about it. The agent comes from the enrolment act, which is also what
// opened the grid the frame is read from.
type Enrolment interface {
	// AgentOn names the agent, and false for a pane nocx is not watching.
	// False is the ordinary answer: almost no pane is enrolled.
	AgentOn(paneID string) (string, bool)
}

// Input is the pane's own input queue — the same one every keystroke goes to,
// so a submission is subject to everything a keystroke is subject to: the
// bootstrap window's quarantine, the queue's bound, this session and no other.
type Input interface {
	// Accept queues b as this pane's input and reports whether the queue took
	// it. ACCEPTED IS NOT DELIVERED: true means the queue has the job, and the
	// write to the pty happens later and can still fail.
	Accept(paneID string, b []byte) bool
}

// Outcome is the closed set of things that can happen to a submission. Three
// values, because "the text landed and the submit key did not" is a real state
// of the world and folding it into either neighbour would lie about it.
type Outcome string

const (
	// OutcomeSubmitted means both segments were accepted: the text is in the
	// pane's input queue and so is the key that submits it.
	OutcomeSubmitted Outcome = "submitted"
	// OutcomeTyped means the text was accepted and no submit key was sent —
	// either because none was asked for, or because the screen stopped being
	// free_text between the two. The person sees the text in the input region
	// and can send or clear it; that is the recoverable half of the only
	// partial state this has.
	OutcomeTyped Outcome = "typed"
	// OutcomeRefused means nothing at all was written.
	OutcomeRefused Outcome = "refused"
)

// Result is what happened, and it has the same shape whether nocx typed or
// refused. A refusal is an ANSWER rather than an absence: a control that
// silently does nothing is indistinguishable from a broken one, and a refusal
// nobody can read is how this degrades into typing blindly.
type Result struct {
	// PaneID is the pane the attempt was about.
	PaneID string
	// Agent is who the enrolment act said is running there. Empty for a pane
	// nocx is not watching, which is the one case where there is nobody to
	// name.
	Agent string
	// Outcome is the closed-set answer.
	Outcome Outcome
	// State is the state that DECIDED. free_text when bytes were written;
	// otherwise the state that refused them. StateUnknown when nothing could
	// read the screen at all — no rule with authority, no grid, no enrolment —
	// which is the same value every other consumer treats as busy.
	//
	// A REFUSAL CAN NAME free_text, and that is not a contradiction: the box a
	// line would be appended to is not a member of the driver's closed set, so
	// a pane that IS waiting for input but holds somebody's draft refuses with
	// the state it actually read (nocx-xn63t.6.44). A caller that treats
	// State == free_text as "the write went through" would be reading the
	// Outcome field, which is the one that answers that question.
	State agentdriver.State
	// Reason is why, in the words a person reads. Empty exactly when the whole
	// submission was accepted.
	Reason string
}

// Typist is the one thing in nocx that writes into an agent's pane.
type Typist struct {
	log      log.Logger
	screens  Screens
	rules    Rules
	calib    Authority
	enrolled Enrolment
	input    Input

	// factsMu guards the two per-pane memories below, and it is here rather
	// than beside the seams because they are the only mutable state this type
	// has: everything else is handed to it once and never written again. One
	// lock for both, because they are read and written by the same calls and
	// neither is ever held across a frame read or a queue write.
	factsMu sync.Mutex
	// refusalReason is the reason each pane's last refusal was REPORTED for
	// (nocx-xn63t.6.52). See warn for why a repeat is not a line.
	refusalReason map[string]string
	// ownBytes is, per pane, the line this package pasted into that pane's box
	// and could NOT submit (nocx-xn63t.6.53). It is what makes the next grant
	// able to tell nocx's own bytes from a person's draft when the box is not
	// empty, and it is dropped the moment the line leaves the box.
	ownBytes map[string]string
}

// maxPaneFacts bounds BOTH per-pane memories — the reason each pane's last
// refusal was reported for, and the line each pane's box was left holding — at
// the same number that bounds the panes nocx may watch, which is the only thing
// that can appear in either of them. One bound for both because it is one
// question: a pane, and at most how many there are. Entries for panes that have
// since gone are the only way past it, so a map at the bound is DROPPED WHOLE
// rather than grown: losing the memory re-reports one refusal per pane, and
// forgets which boxes nocx's own lines stand in — both the harmless direction,
// since the first is a line a person has already read and the second refuses,
// which is what the gate does without it — while a map keyed by session ids and
// never pruned would outlive every pane it ever described.
const maxPaneFacts = paneview.MaxWatched

// New wires the typing seam at the composition root.
func New(lg log.Logger, screens Screens, rules Rules, calib Authority, enrolled Enrolment, input Input) *Typist {
	return &Typist{log: lg, screens: screens, rules: rules, calib: calib, enrolled: enrolled, input: input}
}

// Submit puts text into a pane's input and then presses the submit key, as two
// separate writes each gated on its own frame.
func (t *Typist) Submit(ctx context.Context, paneID, text string) Result {
	return t.put(ctx, paneID, text, true)
}

// Type puts text into a pane's input and presses nothing. It is the same
// primitive with the second segment left off: the text appears in the input
// region and answers nothing, which is what a person confirming that a rule
// they just calibrated actually works needs, and what a caller composing a
// submission a human will send needs.
func (t *Typist) Type(ctx context.Context, paneID, text string) Result {
	return t.put(ctx, paneID, text, false)
}

func (t *Typist) put(ctx context.Context, paneID, text string, submit bool) Result {
	body, err := bodyOf(text)
	if err != nil {
		// Before the grant, because a submission nocx cannot send is not a
		// question about the screen. There is no agent to name yet either.
		return *t.refuse(paneID, "", agentdriver.StateUnknown, err.Error())
	}
	p, refusal := t.grant(ctx, paneID, submit)
	if refusal != nil {
		return *refusal
	}
	if p.ownLine {
		// THE LINE IN THE BOX IS ALREADY NOCX'S (nocx-xn63t.6.53). What it owes
		// is the submit key it was refused, and pressing that is the whole of
		// this attempt: a second paste would put a second line behind the first
		// and submit both.
		return t.finishOwnLine(ctx, p)
	}
	if r := p.write(ctx, []byte(pasteStart+body+pasteEnd), boxEmpty); r != nil {
		return *r
	}
	// The pane took the text, so whatever refused before this has stopped
	// being true (see wrote).
	t.wrote(p.pane)
	typed := Result{
		PaneID: p.pane, Agent: p.agent,
		Outcome: OutcomeTyped, State: agentdriver.StateFreeText,
	}
	if !submit {
		t.log.Info("nocx typed into a pane", "pane_id", p.pane, "agent", p.agent, "bytes", len(body))
		return typed
	}
	// The submit key is gated on its own frame but NOT on the box: the text
	// this call just pasted is in it, and a box check here would refuse the key
	// of every submission.
	if r := p.write(ctx, []byte(submitKey), boxIgnored); r != nil {
		// The one partial state this has, and it is named rather than folded
		// into either neighbour: the text is in the input region and nothing
		// answered anything. Better than the alternative by exactly the margin
		// this package exists for — the alternative is a submit key landing on
		// a screen that is no longer free text.
		//
		// AND THE BYTES ARE NOW NOCX'S TO FINISH (nocx-xn63t.6.53): they are
		// recorded against the pane, so the next grant can tell this line from a
		// person's draft and press the key that was refused instead of refusing
		// for as long as they stand. The reason says whose they are, because a
		// person reading the record cannot see the box.
		typed.Reason = r.Reason + ownLineClause
		typed.State = r.State
		t.rememberOwnLine(p.pane, body)
		return typed
	}
	t.forgetOwnLine(p.pane)
	t.log.Info("nocx submitted into a pane", "pane_id", p.pane, "agent", p.agent, "bytes", len(body))
	return Result{
		PaneID: p.pane, Agent: p.agent,
		Outcome: OutcomeSubmitted, State: agentdriver.StateFreeText,
	}
}

// permit is the authority to put bytes into ONE pane.
//
// grant is the only thing that builds a non-zero one, and it does so in a
// single statement after BOTH gates have answered. So a permit a later edit
// builds by hand carries no Typist, and write refuses on that — there is no
// boolean to set and no pane to point it at, because the pane is inside the
// permit and the only door to the input queue is `by`.
type permit struct {
	by    *Typist
	pane  string
	agent string
	// ownLine is that the box this permit is minted on already holds the line
	// this package put there and could not submit (nocx-xn63t.6.53), so the
	// write this permit authorises is the submit key and not a paste. It is
	// set here and nowhere else, and it can only be true for a call that
	// SUBMITS: Type never sent a key to be owed, and its text would be
	// appended to the standing line.
	ownLine bool
}

// grant asks every gate and mints the permit, or answers why not.
//
// The order is deliberate. Authority first, because classifying with a rule
// that has not earned the right to be believed produces a state nobody should
// act on — so an unverified rule refuses at StateUnknown, which is the honest
// answer (without a verified rule nocx does not know what the pane is) and is
// what every other consumer already treats as busy.
func (t *Typist) grant(ctx context.Context, paneID string, submit bool) (permit, *Result) {
	agent, refusal := t.authority(ctx, paneID)
	if refusal != nil {
		return permit{}, refusal
	}
	o, lookRefusal := t.look(paneID, agent)
	if lookRefusal != nil {
		return permit{}, lookRefusal
	}
	// THE BOX IS THE THIRD GATE, AND IT IS NOT PART OF look (nocx-xn63t.6.44).
	//
	// free_text says the pane is waiting for input; it does not say the box is
	// empty. A line written into a box that already holds a draft is APPENDED
	// to it and submitted with it, so the person's half-written question goes
	// to their agent with nocx's sentence in front of it — measured 2026-10-18,
	// on a coordinator whose owner had started typing.
	//
	// It is asked HERE, when the permit is minted, and once more in write on
	// the frame that authorises the TEXT — the same interval discipline the
	// state gate already follows, so a keystroke landing between the two reads
	// still stops the paste. It is deliberately NOT asked on the submit key:
	// by then the box holds the text this very call just pasted, and a box gate
	// on that segment would refuse the submit key of every submission this
	// package makes. It is not part of look for the same reason.
	//
	// The window that leaves after the last read is the same residual the state
	// gate names: a person's own keystrokes racing ours to one pty.
	//
	// A BOX NOCX FILLED ITSELF IS NOT THAT DRAFT (nocx-xn63t.6.53). A paste
	// this package made and could not submit leaves its own line standing, and
	// the empty-box rule read against it refuses every later wake for as long as
	// it stands — a wake blocked by nocx's own bytes, which nothing clears.
	// So a SUBMITTING call is admitted when the rule reads back exactly the
	// bytes this package last left in that box, and what it then presses is the
	// submit key. Equality is the whole of the licence: a person's sentence,
	// their sentence with our line in front of it, and our line edited by a
	// keystroke all read differently and are refused as before.
	ownLine := submit && holdsOwnLine(o, t.ownLine(paneID))
	if !o.BoxEmpty() && !ownLine {
		return permit{}, t.refuse(paneID, agent, o.State, emptyBoxRefusal)
	}
	// Every answer is in. This is the only statement in this package that
	// builds a permit with a Typist in it.
	return permit{by: t, pane: paneID, agent: agent, ownLine: ownLine}, nil
}

// finishOwnLine presses the submit key on the line THIS package left in the
// pane's box, and answers the submission the earlier attempt owed
// (nocx-xn63t.6.53).
//
// It writes one byte and nothing else. The key is gated by write exactly as it
// was the first time — on a frame read microseconds before it, from a pane the
// grant has just found free_text and vouched for — so a screen that has moved
// again refuses again, and the record of our line stands for the next attempt
// rather than being spent on a refusal.
//
// WHAT IS SUBMITTED IS THE LINE ALREADY IN THE BOX, not the text this call was
// given, and the difference is a count that may have moved: a wake names how
// much mail is waiting, and mail that arrived between the two attempts is not in
// a line that is already typed. It is the small difference and it is the right
// one — the line is a pointer at workers.inbox rather than a copy of the mail,
// the coordinator reads the truth when it calls, and the alternative is nocx's
// own bytes blocking the wake for as long as they stand, which is the whole
// defect. Nothing here can rewrite the text without clearing the box first, and
// clearing it means sending keys that no measured fact says this agent binds.
func (t *Typist) finishOwnLine(ctx context.Context, p permit) Result {
	left := t.ownLine(p.pane)
	if r := p.write(ctx, []byte(submitKey), boxIsOwnLine); r != nil {
		r.Reason += ownLineClause
		return *r
	}
	// The line has left the box with the turn it starts, so the record of it is
	// spent: the same characters turning up later are a person's.
	t.forgetOwnLine(p.pane)
	t.wrote(p.pane)
	t.log.Info("nocx pressed the submit key on its own line that the pane's box still held",
		"pane_id", p.pane, "agent", p.agent, "bytes", len(left))
	return Result{
		PaneID: p.pane, Agent: p.agent,
		Outcome: OutcomeSubmitted, State: agentdriver.StateFreeText,
	}
}

// holdsOwnLine answers whether this frame's box holds EXACTLY the line nocx
// itself left in it: the rule read the box, and what it read is our bytes.
//
// It is asked only of a box that is NOT empty, and only for a call that
// submits. false covers every way the question can be answered "not ours" — an
// empty standing record, a rule that cannot read this box (a menu has displaced
// it), and a box holding anything but our line — which is the direction that
// refuses.
func holdsOwnLine(o agentdriver.Observation, own string) bool {
	if own == "" {
		return false
	}
	text, ok := o.InputText()
	if !ok {
		return false
	}
	return sameBoxText(text, own)
}

// sameBoxText compares what the rule read in the box with the bytes this
// package pasted.
//
// The comparison is on WHITESPACE-COLLAPSED text and not on the raw strings,
// because a box's reading is a DRAWING of the text and not the text: the rule
// joins the content rows it read with the separator its agent needs (claude's
// is a space, measured off a wrapped box), and a line that wrapped at a space
// therefore reads back with the same words and different runs of whitespace.
// Collapsing runs is the smallest tolerance that covers it, and it cannot widen
// who matches: a person's draft differs from our line in its words, not in the
// width of a space, and one that differed in nothing else would be our line.
func sameBoxText(box, own string) bool {
	collapse := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	return collapse(box) == collapse(own)
}

// ownLine is the line this pane's box was left holding and could not submit, or
// the empty string when there is none.
func (t *Typist) ownLine(paneID string) string {
	t.factsMu.Lock()
	defer t.factsMu.Unlock()
	return t.ownBytes[paneID]
}

// rememberOwnLine records the bytes this package pasted and could not submit.
func (t *Typist) rememberOwnLine(paneID, text string) {
	t.factsMu.Lock()
	defer t.factsMu.Unlock()
	if t.ownBytes == nil {
		t.ownBytes = make(map[string]string, 1)
	} else if len(t.ownBytes) >= maxPaneFacts {
		clear(t.ownBytes)
	}
	t.ownBytes[paneID] = text
}

// forgetOwnLine drops that record once the line has left the box — submitted,
// or replaced by a paste that went through — so a person's text that happens to
// read the same is never treated as nocx's licence.
func (t *Typist) forgetOwnLine(paneID string) {
	t.factsMu.Lock()
	defer t.factsMu.Unlock()
	delete(t.ownBytes, paneID)
}

// authority is the half of a grant that is not about the screen: the pane is
// enrolled, and its agent's rule has earned the right to be believed.
func (t *Typist) authority(ctx context.Context, paneID string) (string, *Result) {
	agent, watched := t.enrolled.AgentOn(paneID)
	if !watched {
		return "", t.refuse(paneID, "", agentdriver.StateUnknown,
			"nocx is not watching that pane, so there is no rule to ask about it")
	}
	if v := t.calib.Verify(ctx, agent); !v.MayType() {
		reason := v.Reason
		if reason == "" {
			reason = fmt.Sprintf("%s's rule has not earned the right to be typed against", agent)
		}
		return "", t.refuse(paneID, agent, agentdriver.StateUnknown, reason)
	}
	return agent, nil
}

// look reads the pane's screen NOW and answers whether nocx may write into it.
//
// One derivation of "is this pane inviting input", read by the grant and by
// every write. A second one would agree everywhere anybody looked and disagree
// on the frame nobody tried, which is the shape this repository has paid for.
func (t *Typist) look(paneID, agent string) (agentdriver.Observation, *Result) {
	f, err := t.screens.Frame(paneID)
	if err != nil {
		// The ordinary race: the session ended and the watch closed. A pane
		// with no screen is not a pane with an idle screen.
		return agentdriver.Observation{}, t.refuse(paneID, agent, agentdriver.StateUnknown,
			"nocx has no live screen for that pane, so it cannot see what typing would answer")
	}
	if refusal := t.vouched(paneID, agent, f); refusal != nil {
		return agentdriver.Observation{}, refusal
	}
	// ONE evaluation of this frame, and both answers come out of it: the state
	// that decides whether nocx may type here at all, and whether the box it
	// would type into is empty — grant asks the second of the permit's frame
	// and write asks it again of the paste's. Asking the registry twice would be
	// two readings of one frame that could disagree; Classify is Observe's own
	// scalar projection for exactly that reason.
	o := t.rules.Observe(agent, f)
	if o.State != agentdriver.StateFreeText {
		return o, t.refuse(paneID, agent, o.State,
			fmt.Sprintf("that pane is %s, and nocx types only into a pane that is waiting for input", said(o.State)))
	}
	return o, nil
}

// vouched is the completeness gate: the reading a write is decided on must be
// one the runtime can vouch for.
//
// It sits beside every read a WRITE is decided from, and not on the reads an
// OBSERVATION is, because the two answer different questions. An indicator
// drawn from a screen with a hole is a hint that may be wrong; a keystroke sent
// on such a screen is an answer to a question nobody can prove is on it — and
// the hole is precisely the case where the cells no longer describe the
// program's state (design §6.7: "while it is unknown the write gate refuses and
// the attach says so").
//
// Every claim that is not Complete is refused, and Evicted is worth naming
// because it is the one a reader might expect to be exempt: it means retention
// kept DELIBERATELY less than the whole, so nothing was lost by accident — and
// the screen it describes is still not the whole stream. A write decided on it
// would be an answer to a question half of whose context was thrown away on
// purpose, which is no better than one decided on a hole.
//
// A replay is Complete by construction and a byte-zero session runtime says so
// too, so this refuses nothing an ordinary pane can produce.
func (t *Typist) vouched(paneID, agent string, f paneview.Frame) *Result {
	if f.Completeness == sessionruntime.CompletenessComplete {
		return nil
	}
	return t.refuse(paneID, agent, agentdriver.StateUnknown, fmt.Sprintf(
		"nocx cannot vouch for what this pane's screen holds (%s), so nothing was written",
		saidCompleteness(f.Completeness)))
}

// saidCompleteness names what a runtime claims, in words a person reading a
// refusal can act on. Every value is named rather than only the ones this
// build expects: a claim that arrived from a newer helper must not be described
// as the thing it is not.
func saidCompleteness(c sessionruntime.Completeness) string {
	switch c {
	case sessionruntime.CompletenessUnknown:
		return "the runtime cannot say whether it saw the whole stream"
	case sessionruntime.CompletenessLostIngest:
		return "output was lost before the screen was built"
	case sessionruntime.CompletenessNoFence:
		return "the interval has no authenticated boundary"
	case sessionruntime.CompletenessEvicted:
		return "retention deliberately kept less than the whole"
	case sessionruntime.CompletenessComplete:
		return "the whole stream reached the screen"
	default:
		return "the runtime reported a completeness this build does not know"
	}
}

// boxRule is what the frame that AUTHORISES a write must show in the pane's
// input box. Three values, because the three writes this package makes need
// three different answers and a boolean could only tell two of them apart.
type boxRule int

const (
	// boxIgnored is the submit key of an ordinary submission, which is gated on
	// the pane's state and not on the box: the paste that precedes it is in the
	// box by then, and a box check there would refuse the key of every
	// submission this package makes.
	boxIgnored boxRule = iota
	// boxEmpty is the text, which may only go into an EMPTY box: anything else
	// in it is somebody's draft, and appending to that is the defect.
	boxEmpty
	// boxIsOwnLine is the submit key pressed on the line this package pasted and
	// could not submit (nocx-xn63t.6.53): the box must still hold EXACTLY those
	// bytes, so a person's keystroke landing between the grant and this read
	// stops the key rather than submitting their words with ours.
	boxIsOwnLine
)

// write re-takes the decision from a frame read HERE and writes b only if that
// frame still says free_text — and still says of the BOX what this write's own
// rule requires of it. Nothing stands between the read and the queue call: b was
// built before the permit was asked for.
//
// The rule is boxIgnored for exactly one write: the submit key of an ordinary
// submission, whose own paste is already in the box by the time that key is
// gated. The submit key pressed on nocx's OWN line is not that write — there the
// box is the whole of what makes the key safe — and it is checked.
func (p permit) write(_ context.Context, b []byte, rule boxRule) *Result {
	if p.by == nil {
		// The zero permit. Unreachable while grant is the only constructor,
		// and stated anyway: an invariant held only by a caller's good
		// behaviour is held by nobody.
		return &Result{
			PaneID: p.pane, Outcome: OutcomeRefused, State: agentdriver.StateUnknown,
			Reason: "nothing granted this write, so nothing was written",
		}
	}
	o, refusal := p.by.look(p.pane, p.agent)
	if refusal != nil {
		return refusal
	}
	switch rule {
	case boxEmpty:
		if !o.BoxEmpty() {
			// The person's keystroke landed between the permit and this read.
			// The paste is not sent: appending to a draft IS the defect, and it
			// is the same defect at any window width.
			return p.by.refuse(p.pane, p.agent, o.State, emptyBoxRefusal)
		}
	case boxIsOwnLine:
		if !holdsOwnLine(o, p.by.ownLine(p.pane)) {
			// The box is no longer only ours: a keystroke, or the agent's own
			// drawing, put something else there. The key is not pressed, and
			// the record of our line stays — it is the next grant's business
			// whether the box still reads as nocx's.
			return p.by.refuse(p.pane, p.agent, o.State, ownLineMovedRefusal)
		}
	}
	if !p.by.input.Accept(p.pane, b) {
		return p.by.refuse(p.pane, p.agent, agentdriver.StateFreeText,
			"that pane is not accepting input at the moment, so nothing was written")
	}
	return nil
}

// refuse records the refusal WITH ITS REASON and returns it. Both, always: the
// log is what a person reads after the fact and the result is what the caller
// shows them now, and a soft degrade visible only in a log is how a feature
// that does not exist survives a release.
func (t *Typist) refuse(paneID, agent string, state agentdriver.State, reason string) *Result {
	t.warn(paneID, agent, state, reason)
	return &Result{PaneID: paneID, Agent: agent, Outcome: OutcomeRefused, State: state, Reason: reason}
}

// warn writes the refusal to the log ONCE PER CHANGE OF REASON (nocx-xn63t.6.52).
//
// A refused attempt is retried on the next settled reading, and a coordinator's
// pane is read once a second (transport.DefaultPaneObserverSweep) — so a
// situation that STANDS, which is the whole point of refusing it, used to write
// the same sentence sixty times a minute. Measured on the owner's stand of
// 2026-10-18: about sixty-five identical lines, and a worker's mail unread
// underneath them. A line that repeats is a line a person learns to skip, which
// costs exactly the refusal somebody needed to read.
//
// What is worth a line is the CHANGE. The same pane refusing for the same
// reason is one fact however many times it is asked; a different reason is
// news, and so is the same reason after a write that went through (see wrote).
// That is why this is not a rate limit: a rate limit would swallow a NEW
// situation that arrived inside the window, while this compares the reason
// itself, so nothing new is ever suppressed.
//
// The result a caller gets is unchanged: the refusal is returned on every
// attempt, because the caller acts on it and the log only records it.
func (t *Typist) warn(paneID, agent string, state agentdriver.State, reason string) {
	t.factsMu.Lock()
	if t.refusalReason == nil {
		t.refusalReason = make(map[string]string, 1)
	} else if len(t.refusalReason) >= maxPaneFacts {
		clear(t.refusalReason)
	}
	// Exact equality, and not a prefix and not the state: the reason is the
	// whole of what changed, and two refusals that READ the same are one
	// situation a person cannot act on twice.
	if last, reported := t.refusalReason[paneID]; reported && last == reason {
		t.factsMu.Unlock()
		return
	}
	t.refusalReason[paneID] = reason
	t.factsMu.Unlock()
	t.log.Warn("nocx refused to type into a pane",
		"pane_id", paneID, "agent", agent, "state", string(state), "reason", reason)
}

// wrote ends the refusal episode for a pane whose bytes the queue ACCEPTED.
//
// The memory exists to suppress a repeat of a situation that still stands, and
// a write that went through says it no longer stands — so a later refusal in
// the same words is a new stall and must be reported as one. Without this a
// coordinator that stalled twice in a process's life would say so once.
func (t *Typist) wrote(paneID string) {
	t.factsMu.Lock()
	delete(t.refusalReason, paneID)
	t.factsMu.Unlock()
}

// said names a state in the words a person reads. The closed set is the
// driver's; this is the only place it is turned into prose, so a state added
// there arrives here as its own name rather than as a second vocabulary that
// disagrees.
func said(s agentdriver.State) string {
	switch s {
	case agentdriver.StatePermissionChoice:
		return "asking you to approve something"
	case agentdriver.StateModalChoice:
		return "showing a menu"
	case agentdriver.StateWorking:
		return "working"
	case agentdriver.StateError:
		return "showing an error of its own"
	case agentdriver.StateExited:
		return "no longer running"
	default:
		return "in a state nocx cannot read"
	}
}

// bodyOf is what may be typed, and it REFUSES rather than repairs.
//
// A control character is not a character: inside a bracketed paste, an escape
// ends the paste early and everything after it is keystrokes. Stripping one
// would send a submission nobody wrote, so the whole thing is refused and the
// caller is told which byte did it. Carriage returns are the exception the
// other way: a text with \r\n line endings is ordinary, and \r is the submit
// key, so the endings are normalised to \n — a line break inside a bracketed
// paste is content.
func bodyOf(text string) (string, error) {
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("that text is not valid UTF-8, so nocx cannot type it")
	}
	if len(text) > MaxText {
		return "", fmt.Errorf("that text is %d bytes and nocx types at most %d in one submission",
			len(text), MaxText)
	}
	body := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("there is nothing to type")
	}
	for _, r := range body {
		if r == '\n' || r == '\t' {
			continue
		}
		// C0, DEL, and C1 — the last of which is the one worth naming: a
		// terminal decoding UTF-8 reads U+009B as CSI, so an escape sequence
		// can be spelled in two bytes that are not the byte 0x1b and would
		// pass a check that only looked below 0x20.
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return "", fmt.Errorf(
				"that text carries a control character (%#U), which inside a paste stops being text and starts being keys",
				r)
		}
	}
	return body, nil
}
