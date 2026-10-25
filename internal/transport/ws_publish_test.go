package transport

// The pane's delivery fact, at both ends of the wire (nocx-1kejz.4).
//
// The three checks contracts/README.md asks of a new notification, in the order
// that file gives them: the DTO the transport owns marshals to something the
// contract accepts, every member of the Go vocabulary is a word the contract
// lets through, and the REAL fact — driven through the shipped open path rather
// than assembled by this test — reaches a renderer off the socket.

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/shellintegration"
	"github.com/shady2k/nocx/internal/ssh"
)

// publishFailureReasonVocabulary is every member the Go side can put on the
// wire, listed by hand so a member added to the type and forgotten here is a
// test failure rather than a silently unchecked value. The count assertion
// below is what makes the list's own completeness testable: a vocabulary that
// shrank would otherwise pass by walking nothing.
var publishFailureReasonVocabulary = []shellintegration.PublishFailureReason{
	shellintegration.PublishFailureConnection,
	shellintegration.PublishFailureHome,
	shellintegration.PublishFailureFileTransfer,
	shellintegration.PublishFailureWrite,
	shellintegration.PublishFailureUnknown,
}

func TestSessionPublishFailed_DTOConformsToContract(t *testing.T) {
	schema := loadSchema(t, "session.publishFailed.schema.json")
	if len(publishFailureReasonVocabulary) < 5 {
		t.Fatalf("only %d publish failure reasons are listed; every one this backend can send is a "+
			"case the contract must accept", len(publishFailureReasonVocabulary))
	}
	for _, reason := range publishFailureReasonVocabulary {
		t.Run(string(reason), func(t *testing.T) {
			raw, err := json.Marshal(publishFailureParams{
				SessionID: "session-1", InstanceID: "instance-1", SessionEpoch: 1,
				Reason: string(reason),
			})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			validateJSON(t, schema, raw, string(reason)+" DTO")
		})
	}
}

// A reason a person can act on is the whole point of the closed set: the raw
// error never travels (nocx-viil), so the contract must refuse a sentence that
// is not one of its words. This is the negative half of the check above.
func TestSessionPublishFailed_RefusesAPayloadOutsideItsContract(t *testing.T) {
	schema := loadSchema(t, "session.publishFailed.schema.json")
	for name, params := range map[string]publishFailureParams{
		"the raw error's own words": {
			SessionID: "session-1", InstanceID: "instance-1", SessionEpoch: 1,
			Reason: "Permission denied (publickey).",
		},
		"an empty reason": {
			SessionID: "session-1", InstanceID: "instance-1", SessionEpoch: 1,
		},
		"an epoch that is not one": {
			SessionID: "session-1", InstanceID: "instance-1", SessionEpoch: 0,
			Reason: string(shellintegration.PublishFailureWrite),
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(params)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := validateJSONErr(schema, raw); err == nil {
				t.Fatalf("the contract accepted %s", raw)
			}
		})
	}
}

// THE VOCABULARY MAY NOT OUTGROW THE WIRE.
//
// The same defect TestEveryRefusalReasonIsDeclaredOnTheWire guards against in
// internal/app: a member added to the Go set and not to the contract's enum is
// a fact the schema REJECTS, so the product shows nothing at all — the soft
// degrade the schema's own description warns about, wearing the opposite hat.
// Go has no reflection over a package's constants, so the declaration is
// parsed: the authority is read, never restated.
func TestEveryPublishFailureReasonIsDeclaredOnTheWire(t *testing.T) {
	wire := map[string]bool{}
	for _, reason := range publishFailureReasonVocabulary {
		wire[string(reason)] = true
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../shellintegration/publish_failure.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse the publish failure vocabulary: %v", err)
	}

	found := 0
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
			return true
		}
		id, ok := vs.Type.(*ast.Ident)
		if !ok || id.Name != "PublishFailureReason" {
			return true
		}
		lit, ok := vs.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value := strings.Trim(lit.Value, `"`)
		found++
		// PublishFailureNone is the empty string — "nothing to report" — and is
		// deliberately not a wire value: the notification is absent instead.
		if value == "" {
			return true
		}
		if !wire[value] {
			t.Errorf("shellintegration.%s = %q is not in the contract's closed reason enum. "+
				"A session refused for this reason would have its notification rejected at the "+
				"schema, so the product would show nothing at all — add it to "+
				"contracts/session.publishFailed.schema.json and to the renderer's sentences",
				vs.Names[0].Name, value)
		}
		return true
	})

	if found < 6 {
		t.Fatalf("only %d PublishFailureReason constants were parsed; the vocabulary is larger than "+
			"that, so this test is not reading what it thinks it is reading", found)
	}
}

// publishFailureOpener is the shipped open route with one fact varied: the
// opener reports an attempt whose copy did not land, which is what
// internal/app's opener does when the bundle write fails.
type publishFailureOpener struct {
	reg    *session.Reg
	id     session.ID
	reason shellintegration.PublishFailureReason
}

func (o *publishFailureOpener) OpenHosted(ctx context.Context, cfg session.Config, _ string) (HostedSessionOpen, bool, error) {
	sess, err := o.reg.Adopt(ctx, cfg, o.id, &helperOpenTestChannel{done: make(chan struct{})})
	if err != nil {
		return HostedSessionOpen{}, false, err
	}
	return HostedSessionOpen{
		Session: sess, Host: cfg.Host, Account: "alice", Generation: "gen-test",
		PublishFailure: o.reason,
	}, true, nil
}

// openPublishFailurePane opens one ssh pane through the shipped handler and
// answers the session id the helper minted, once the open's subscriber is
// installed — the state every fact about that session is emitted into.
func openPublishFailurePane(t *testing.T, ws *WSServer, conn *websocket.Conn, id session.ID) string {
	t.Helper()
	resp := jsonrpcCall(t, conn, "open", map[string]any{
		"kind": "ssh", "profileId": "p", "cols": 80, "rows": 24, "paneId": helperOpenPane,
	})
	var opened struct {
		SessionID string `json:"sessionId"`
	}
	decodeJSONRPCResult(t, resp, &opened)
	if opened.SessionID != string(id) {
		t.Fatalf("open sessionId = %q, want helper-minted %q", opened.SessionID, id)
	}
	awaitSubscriber(t, ws, id)
	return opened.SessionID
}

// TestSessionPublishFailed_OverTheWireConformsToContract drives a real open
// through the shipped handler and reads the fact off the socket. A test that
// validated a payload it built itself would prove the struct is well-formed,
// not that the server sends it — which is contracts/README.md's whole argument
// for this shape of check.
func TestSessionPublishFailed_OverTheWireConformsToContract(t *testing.T) {
	schema := loadSchema(t, "session.publishFailed.schema.json")
	logger := log.NewSlogAdapter(nil)
	reg := newRegWithStub(logger)
	helperID := session.ID("0123456789abcdef0123456789abcde1")
	ws := NewWSServer(logger, reg,
		WithProfileResolver(&fakeResolver{
			resolveFn: func(string) (string, *ssh.ConnectConfig, error) {
				return "remote.example", &ssh.ConnectConfig{User: "alice"}, nil
			},
		}),
		WithHelperSessionOpener(&publishFailureOpener{
			reg: reg, id: helperID, reason: shellintegration.PublishFailureFileTransfer,
		}),
	)
	ctx := context.Background()
	if err := ws.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = ws.Stop(ctx) }()
	conn := connectWS(t, ws)
	defer func() { _ = conn.Close() }()

	sid := openPublishFailurePane(t, ws, conn, helperID)

	raw, err := awaitNotification(conn, "session.publishFailed", wantWithin)
	if err != nil {
		t.Fatalf("waiting for the delivery-failure notification: %v", err)
	}
	validateJSON(t, schema, raw, "session.publishFailed wire")
	var got publishFailureParams
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SessionID != sid || got.Reason != string(shellintegration.PublishFailureFileTransfer) {
		t.Fatalf("wire fact = %+v, want session %q with reason %q", got, sid,
			shellintegration.PublishFailureFileTransfer)
	}
	if got.InstanceID == "" || got.SessionEpoch == 0 {
		t.Fatalf("wire fact carries no identity: %+v — a replayed fact about a previous incarnation "+
			"of this session id would be applied to the wrong pane", got)
	}
}

// And the other end of the pair: a pane whose copy LANDED is told nothing. A
// notification per successful ssh open would put a card in front of every
// healthy pane, which is the failure mode that makes people stop reading cards.
func TestSessionPublishFailed_IsSilentWhenTheCopyLanded(t *testing.T) {
	logger := log.NewSlogAdapter(nil)
	reg := newRegWithStub(logger)
	helperID := session.ID("0123456789abcdef0123456789abcde2")
	ws := NewWSServer(logger, reg,
		WithProfileResolver(&fakeResolver{
			resolveFn: func(string) (string, *ssh.ConnectConfig, error) {
				return "remote.example", &ssh.ConnectConfig{User: "alice"}, nil
			},
		}),
		WithHelperSessionOpener(&publishFailureOpener{
			reg: reg, id: helperID, reason: shellintegration.PublishFailureNone,
		}),
	)
	ctx := context.Background()
	if err := ws.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = ws.Stop(ctx) }()
	conn := connectWS(t, ws)
	defer func() { _ = conn.Close() }()

	openPublishFailurePane(t, ws, conn, helperID)

	// SILENCE, ASSERTED AS A STATE RATHER THAN AS AN ABSENCE OF TRAFFIC. The
	// retained map is the only source of this notification — emitPublishFailure
	// reads it and returns when there is no entry — so "nothing was retained"
	// and "nothing was sent" are the same fact here, and the test above proves
	// the send path from that map really reaches the socket. Waiting a few
	// hundred milliseconds for nothing would be the other way to say it, and it
	// would be a test that passes on a quiet machine and fails on a busy one.
	ws.publishMu.Lock()
	reason, retained := ws.publishFailures[helperID]
	ws.publishMu.Unlock()
	if retained {
		t.Fatalf("a pane whose bundle was copied retained the delivery failure %q, so it would be told "+
			"about a copy that landed", reason)
	}
}
