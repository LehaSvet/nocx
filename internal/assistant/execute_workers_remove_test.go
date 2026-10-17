package assistant

// workers.removeCheckout against its contract (nocx-xn63t.1.5).
//
// The checks here are AGENTS.md rule 5's halves for the new answer: the DTO
// the executor marshals must satisfy the schema's own result shape — a
// removed row (which claims nothing beyond its removal), a refused row
// (which names the refusal and the why), and a row whose ask resolved to
// nothing (path absent, branch present) — and the real result off the real
// dispatcher must satisfy it too, because a payload the test built for
// itself proves nothing about what the wire actually carries.
//
// The refusals themselves are the service's (internal/app, the holdings
// walk); what this surface owes is that they reach the caller VERBATIM, in
// the ask's order, with a removed row carrying no refusal at all.
//
// The discard (nocx-xn63t.6.40) adds the two things this surface does own:
// the ask's OWN flag travels on the ref of the checkout it was asked about
// and nowhere else, and the counts the walk answered reach the wire on the
// removed row — and on no other row, which the schema itself forbids.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/shady2k/nocx/internal/agenttools"
	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/workers"
)

func workerRemoveCheckoutResultSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile("../../contracts/tools/workers.removeCheckout.schema.json")
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	var doc struct {
		Defs map[string]json.RawMessage `json:"$defs"`
	}
	if unmarshalErr := json.Unmarshal(raw, &doc); unmarshalErr != nil {
		t.Fatalf("parse the contract: %v", unmarshalErr)
	}
	compiler := jsonschema.NewCompiler()
	resource, err := jsonschema.UnmarshalJSON(strings.NewReader(string(doc.Defs["result"])))
	if err != nil {
		t.Fatalf("read $defs/result: %v", err)
	}
	if addErr := compiler.AddResource("workers.removeCheckout.result.json", resource); addErr != nil {
		t.Fatalf("add the result schema: %v", addErr)
	}
	schema, err := compiler.Compile("workers.removeCheckout.result.json")
	if err != nil {
		t.Fatalf("compile the result schema: %v", err)
	}
	return schema
}

// Criterion: every row the executor can produce marshals to something the
// contract accepts — removed (and claiming nothing else), refused by name
// with its why, and unresolvable (no path, the branch that was asked
// about).
func TestWorkerRemoveCheckoutDTOConformsToContract(t *testing.T) {
	cases := []struct {
		name  string
		items []workers.RemovedCheckout
	}{
		{"a removed row", []workers.RemovedCheckout{
			{Path: "/wt/nocx-feat", Branch: "feat/one", Removed: true},
		}},
		{"a refused row", []workers.RemovedCheckout{
			{
				Path: "/wt/nocx-feat", Branch: "feat/one",
				Refusal: workers.CheckoutRefusalUncommitted,
				Detail:  `git: worktree "/wt/nocx-feat" holds 2 uncommitted change(s); nothing was removed`,
			},
		}},
		{"a branch ask that resolved to nothing", []workers.RemovedCheckout{
			{
				Branch:  "feat/nope",
				Refusal: workers.CheckoutRefusalNotOurs,
				Detail:  `no checkout of this repository holds branch "feat/nope", so there is nothing of nocx's to remove`,
			},
		}},
		{"a removed row that discarded work", []workers.RemovedCheckout{
			{
				Path: "/wt/nocx-left", Branch: "feat/left", Removed: true,
				Discarded: &workers.DiscardedWork{Modified: 2, Untracked: 3},
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &fakeWorkerRecord{removal: workers.CheckoutRemoval{Items: tc.items}}
			out, err := executeWorkerRemoveCheckout(context.Background(),
				testCoordinator("sess-coordinator"),
				json.RawMessage(`{"checkouts":[{"path":"/wt/nocx-feat"},{"branch":"feat/nope"}]}`),
				workerSeams(rec))
			if err != nil {
				t.Fatalf("workers.removeCheckout: %v", err)
			}
			var value any
			if unmarshalErr := json.Unmarshal([]byte(out), &value); unmarshalErr != nil {
				t.Fatalf("decode result: %v", unmarshalErr)
			}
			if validateErr := workerRemoveCheckoutResultSchema(t).Validate(value); validateErr != nil {
				t.Fatalf("workers.removeCheckout result does not satisfy its contract: %v\npayload: %s", validateErr, out)
			}
		})
	}
}

// Criterion: a removed row carries NO refusal on the wire — removal and
// refusal in one row would be one field meaning two things.
func TestWorkerRemoveCheckoutARemovedRowClaimsNothingBeyondItsRemoval(t *testing.T) {
	rec := &fakeWorkerRecord{removal: workers.CheckoutRemoval{Items: []workers.RemovedCheckout{
		{Path: "/wt/nocx-feat", Branch: "feat/one", Removed: true},
	}}}
	out, err := executeWorkerRemoveCheckout(context.Background(),
		testCoordinator("sess-coordinator"),
		json.RawMessage(`{"checkouts":[{"path":"/wt/nocx-feat"}]}`), workerSeams(rec))
	if err != nil {
		t.Fatalf("workers.removeCheckout: %v", err)
	}
	var decoded struct {
		Checkouts []map[string]any `json:"checkouts"`
	}
	if unmarshalErr := json.Unmarshal([]byte(out), &decoded); unmarshalErr != nil {
		t.Fatalf("decode result: %v", unmarshalErr)
	}
	if len(decoded.Checkouts) != 1 {
		t.Fatalf("rows = %+v, want one", decoded.Checkouts)
	}
	for _, key := range []string{"refusal", "detail"} {
		if _, present := decoded.Checkouts[0][key]; present {
			t.Fatalf("a removed row answered %q — removal must not wear a refusal's shape", key)
		}
	}
}

// Criterion: the refusals reach the caller verbatim, in the ask's order —
// the executor rewords nothing, because the answer must be the same disk
// the sweep of task 1.6 will refuse on.
func TestWorkerRemoveCheckoutCarriesTheRefusalsVerbatimInTheOrderAsked(t *testing.T) {
	rec := &fakeWorkerRecord{removal: workers.CheckoutRemoval{Items: []workers.RemovedCheckout{
		{Path: "/wt/a", Branch: "feat/a", Removed: true},
		{
			Path: "/wt/b", Branch: "feat/b",
			Refusal: workers.CheckoutRefusalHeldByWorker,
			Detail:  "a live worker holds the checkout at /wt/b; close the worker first",
		},
	}}}
	out, err := executeWorkerRemoveCheckout(context.Background(),
		testCoordinator("sess-coordinator"),
		json.RawMessage(`{"checkouts":[{"path":"/wt/a"},{"path":"/wt/b"}]}`), workerSeams(rec))
	if err != nil {
		t.Fatalf("workers.removeCheckout: %v", err)
	}
	var decoded struct {
		Checkouts []workerRemoveItemResult `json:"checkouts"`
	}
	if unmarshalErr := json.Unmarshal([]byte(out), &decoded); unmarshalErr != nil {
		t.Fatalf("decode result: %v", unmarshalErr)
	}
	if len(decoded.Checkouts) != 2 {
		t.Fatalf("rows = %+v, want one per ask", decoded.Checkouts)
	}
	if !decoded.Checkouts[0].Removed || decoded.Checkouts[0].Path != "/wt/a" {
		t.Fatalf("row 0 = %+v, want the clean removal first", decoded.Checkouts[0])
	}
	if decoded.Checkouts[1].Removed ||
		decoded.Checkouts[1].Refusal != string(workers.CheckoutRefusalHeldByWorker) ||
		decoded.Checkouts[1].Detail == "" {
		t.Fatalf("row 1 = %+v, want the held refusal with its why", decoded.Checkouts[1])
	}
	// AND THE SESSION WAS THE CAPABILITY'S: the walk starts at the
	// coordinator's own session, never at an argument.
	if len(rec.removedFor) != 1 || rec.removedFor[0] != "sess-coordinator" {
		t.Fatalf("the service was asked for %v, want the coordinator's own session", rec.removedFor)
	}
	if len(rec.removedRefs) != 2 || rec.removedRefs[0].Path != "/wt/a" || rec.removedRefs[1].Path != "/wt/b" {
		t.Fatalf("refs = %+v, want the ask carried as named", rec.removedRefs)
	}
}

// Criterion: the record is infrastructure, and its absence is a refusal the
// caller can act on — the same sentence the other worker tools answer with.
func TestWorkerRemoveCheckoutWithoutARecordRefuses(t *testing.T) {
	if _, err := executeWorkerRemoveCheckout(context.Background(),
		testCoordinator("sess-coordinator"),
		json.RawMessage(`{"checkouts":[{"branch":"feat/one"}]}`),
		toolSeams{workerEnvironment: testWorkerEnv}); err == nil {
		t.Fatal("a removal without a record was accepted")
	}
}

// Criterion: a call that bypassed the schema it was shown is refused here —
// an empty ask and an ask that names neither a path nor a branch cannot be
// answered row by row, so they are not answered at all.
func TestWorkerRemoveCheckoutRefusesAnUnanswerableAsk(t *testing.T) {
	for name, raw := range map[string]string{
		"no checkouts at all":  `{"checkouts":[]}`,
		"a row naming neither": `{"checkouts":[{"path":"","branch":""}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := &fakeWorkerRecord{}
			if _, err := executeWorkerRemoveCheckout(context.Background(),
				testCoordinator("sess-coordinator"),
				json.RawMessage(raw), workerSeams(rec)); err == nil {
				t.Fatalf("%s was accepted", name)
			}
			if len(rec.removedFor) != 0 {
				t.Fatalf("the record was asked anyway: %v", rec.removedFor)
			}
		})
	}
}

// Criterion: the discarded counts are what the WALK answered — carried, not
// recomputed — and they ride only the row that removed something. The
// refusal in the same answer threw nothing away, so it names nothing.
func TestWorkerRemoveCheckoutCarriesTheDiscardedCountsOnTheRemovedRowOnly(t *testing.T) {
	rec := &fakeWorkerRecord{removal: workers.CheckoutRemoval{Items: []workers.RemovedCheckout{
		{
			Path: "/wt/nocx-left", Branch: "feat/left", Removed: true,
			Discarded: &workers.DiscardedWork{Modified: 2, Untracked: 3},
		},
		{
			Path: "/wt/nocx-held", Branch: "feat/held",
			Refusal: workers.CheckoutRefusalHeldByWorker,
			Detail:  "a live worker holds the checkout at /wt/nocx-held; close the worker first",
		},
	}}}
	out, err := executeWorkerRemoveCheckout(context.Background(),
		testCoordinator("sess-coordinator"),
		json.RawMessage(`{"checkouts":[{"path":"/wt/nocx-left","discard":true},{"path":"/wt/nocx-held"}]}`),
		workerSeams(rec))
	if err != nil {
		t.Fatalf("workers.removeCheckout: %v", err)
	}
	var decoded struct {
		Checkouts []workerRemoveItemResult `json:"checkouts"`
	}
	if unmarshalErr := json.Unmarshal([]byte(out), &decoded); unmarshalErr != nil {
		t.Fatalf("decode result: %v", unmarshalErr)
	}
	if len(decoded.Checkouts) != 2 {
		t.Fatalf("rows = %+v, want one per ask", decoded.Checkouts)
	}
	first := decoded.Checkouts[0]
	if first.Discarded == nil || first.Discarded.Modified != 2 || first.Discarded.Untracked != 3 {
		t.Fatalf("row 0 = %+v, want the walk's own counts (2 modified, 3 untracked)", first)
	}
	if decoded.Checkouts[1].Discarded != nil {
		t.Fatalf("row 1 = %+v, want no discarded work on a refusal that removed nothing", decoded.Checkouts[1])
	}

	// AND THE FLAG TRAVELLED ON ITS OWN CHECKOUT: the ask discarded one and
	// merely asked about the other, and the service must be able to tell.
	if len(rec.removedRefs) != 2 {
		t.Fatalf("refs = %+v, want one per ask", rec.removedRefs)
	}
	if !rec.removedRefs[0].Discard {
		t.Fatalf("ref 0 = %+v, want the discard the ask named", rec.removedRefs[0])
	}
	if rec.removedRefs[1].Discard {
		t.Fatalf("ref 1 = %+v, want no discard: the ask named one, and it was for the other checkout", rec.removedRefs[1])
	}
}

// Criterion, the interval end other direction: the schema itself refuses a
// refusal that claims discarded work. The wire shape would otherwise permit
// a row saying "nothing was removed, and here is what went" — one row telling
// two stories — and the contract is where that has to be impossible, not the
// executor that happens to build it correctly today.
func TestTheRemoveCheckoutContractRefusesDiscardedWorkOnARefusedRow(t *testing.T) {
	// The value is the WIRE's, decoded the way the dispatcher decodes one
	// (validateToolResult unmarshals the executor's JSON): a hand-built Go
	// value is not the payload, and the schema library's type checker
	// refuses []map[string]any outright — a case that would then pass for a
	// reason that has nothing to do with the clause being tested.
	var value any
	if err := json.Unmarshal([]byte(`{"checkouts":[{`+
		`"path":"/wt/nocx-dirty","branch":"feat/dirty","removed":false,`+
		`"refusal":"uncommitted","detail":"nothing was removed",`+
		`"discarded":{"modified":1,"untracked":0}}]}`), &value); err != nil {
		t.Fatalf("decode the payload: %v", err)
	}
	if err := workerRemoveCheckoutResultSchema(t).Validate(value); err == nil {
		t.Fatal("the contract accepted a refused row claiming discarded work")
	}

	// AND THE OTHER DIRECTION, so the refusal above is the clause and not the
	// row: the same row without the field is what the executor produces and
	// what the contract must accept.
	var clean any
	if err := json.Unmarshal([]byte(`{"checkouts":[{`+
		`"path":"/wt/nocx-dirty","branch":"feat/dirty","removed":false,`+
		`"refusal":"uncommitted","detail":"nothing was removed"}]}`), &clean); err != nil {
		t.Fatalf("decode the clean payload: %v", err)
	}
	if err := workerRemoveCheckoutResultSchema(t).Validate(clean); err != nil {
		t.Fatalf("the contract refused a refusal that claims nothing: %v", err)
	}
}

// Criterion, the third check: the REAL result off the REAL dispatcher —
// schema validation, the narrow, the executor map — satisfies the contract
// when a coordinator removes one checkout and is refused another.
func TestWorkerRemoveCheckout_OverTheWireConformsToContract(t *testing.T) {
	reg, err := agenttools.Assemble(toolsDirFS(t))
	if err != nil {
		t.Fatalf("assemble tools: %v", err)
	}
	rec := &fakeWorkerRecord{removal: workers.CheckoutRemoval{Items: []workers.RemovedCheckout{
		{
			Path: "/wt/nocx-gone", Branch: "feat/gone", Removed: true,
			Discarded: &workers.DiscardedWork{Modified: 1, Untracked: 2},
		},
		{
			Path: "/wt/nocx-dirty", Branch: "feat/dirty",
			Refusal: workers.CheckoutRefusalUncommitted,
			Detail:  `git: worktree "/wt/nocx-dirty" holds 1 uncommitted change(s); nothing was removed`,
		},
	}}}
	dispatcher, err := NewToolDispatcher(reg, rec, content.EnvironmentIDFor(content.EnvLocal, ""))
	if err != nil {
		t.Fatalf("new dispatcher: %v", err)
	}
	invocation := ToolInvocation{
		Context: context.Background(),
		Method:  "workers.removeCheckout",
		RunContext: agenttools.RunContext{
			RunID: "run-1", Session: "sess-coordinator",
		},
		Grant: workerGrantForTest(),
		RawParams: json.RawMessage(
			`{"checkouts":[{"branch":"feat/gone","discard":true},{"path":"/wt/nocx-dirty"}]}`),
	}
	out, dispatchErr := dispatcher.Dispatch(invocation)
	if dispatchErr != nil {
		t.Fatalf("dispatch workers.removeCheckout: %v", dispatchErr)
	}
	var value any
	if unmarshalErr := json.Unmarshal([]byte(out), &value); unmarshalErr != nil {
		t.Fatalf("decode result: %v", unmarshalErr)
	}
	if validateErr := workerRemoveCheckoutResultSchema(t).Validate(value); validateErr != nil {
		t.Fatalf("workers.removeCheckout result off the real dispatcher does not satisfy its contract: %v\npayload: %s", validateErr, out)
	}
}
