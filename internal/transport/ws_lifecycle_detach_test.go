package transport

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/shady2k/nocx/internal/log/logtest"
	"github.com/shady2k/nocx/internal/session"
)

// The registry can only hand a lifecycle over if the entry path armed it.
// Exercise all three entry paths with the real registry, not a setter double.
func TestHostedLifecycleDetachIsArmedBeforeRegistryClose(t *testing.T) {
	for _, entry := range []string{"renderer", "backend", "readopt"} {
		t.Run(entry, func(t *testing.T) {
			ctx, logger := logtest.New(t)
			reg := newRegWithStub(logger)
			opener := &detachCheckingOpener{t: t, reg: reg}
			ws := NewWSServer(logger, reg, WithHelperSessionOpener(opener))
			if err := ws.Start(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ws.Stop(ctx) })
			const sid = session.ID("0123456789abcdef0123456789abcdef")
			switch entry {
			case "renderer":
				conn := connectWS(t, ws)
				t.Cleanup(func() { _ = conn.Close() })
				resp := jsonrpcCall(t, conn, "open", map[string]any{
					"cols": 80, "rows": 24, "paneId": helperOpenPane,
				})
				var result struct {
					SessionID string `json:"sessionId"`
				}
				decodeJSONRPCResult(t, resp, &result)
				if result.SessionID != string(sid) {
					t.Fatalf("open returned session %q, want %q", result.SessionID, sid)
				}
				// The ack precedes lifecycle startup. Wait for the handler to
				// finish installing the session before driving shutdown.
				awaitSubscriber(t, ws, sid)
			case "backend":
				if _, err := ws.OpenSession(ctx, OpenSpec{Cols: 80, Rows: 24, PaneID: helperOpenPane}); err != nil {
					t.Fatal(err)
				}
			case "readopt":
				if err := ws.ReadoptHostedSession(ctx, sid, func(ctx context.Context, _ uint64) (HostedSessionOpen, error) {
					hosted, _, err := opener.OpenHosted(ctx, session.Config{}, "")
					return hosted, err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := reg.Close(sid); err != nil {
				t.Fatal(err)
			}
			if !opener.detached.Load() {
				t.Fatal("registry close returned without the lifecycle handover: the entry path did not arm it")
			}
		})
	}
}

type detachCheckingOpener struct {
	t        *testing.T
	reg      *session.Reg
	detached atomic.Bool
}

func (o *detachCheckingOpener) OpenHosted(ctx context.Context, cfg session.Config, _ string) (HostedSessionOpen, bool, error) {
	sess, err := o.reg.Adopt(ctx, cfg, session.ID("0123456789abcdef0123456789abcdef"), newReadoptChannel(nil))
	if err != nil {
		return HostedSessionOpen{}, true, err
	}
	return HostedSessionOpen{
		Session:        sess,
		StartLifecycle: func() {},
		DetachLifecycle: func() {
			select {
			case <-sess.Done():
				o.t.Error("session carrier closed before lifecycle handover")
			default:
			}
			o.detached.Store(true)
		},
	}, true, nil
}
