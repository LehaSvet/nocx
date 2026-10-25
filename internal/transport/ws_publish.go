package transport

// The pane's "your shell integration was not copied here" fact
// (nocx-1kejz.4).
//
// # What was missing
//
// An ssh pane publishes the integration bundle onto the destination BEFORE it
// asks for the spawn (helper_local.go's openSSH), and a publish that fails is
// deliberately not a refusal: the far side keeps any generation already there,
// byte-identical, and the pane opens as an ordinary terminal (design §6.1 step
// 5, ADR-0004). What the failure did was reach one slog line and stop. The
// person then got a tab that behaved as though delivery had been configured,
// while the tab might be running a stale copy or no integration at all — the
// soft degrade AGENTS.md refuses by name.
//
// # Why a notification, and not a field on the open ack
//
// The ack answers "did a session come into existence" and nothing more
// (nocx-dvql): whatever else it carries can never be revised, and a renderer
// that attaches later — a reloaded window, a second window taking the pane —
// never sees it at all. This is the tool-surface fact's shape for the same two
// reasons: it settles around the open, and it must still be sayable to whoever
// is looking later, so it is RETAINED per session and replayed on attach.
//
// # Why so little travels with it
//
// One reason, from a closed set (internal/shellintegration's
// PublishFailureReason). The raw chain is an SFTP status code and a path:
// meaningful in the log, meaningless in a pane (nocx-viil), and the pane's own
// sentence is the renderer's to write. Success says nothing at all — absence is
// how "there is nothing to tell you" is expressed, the same rule the
// integration axis uses.

import (
	"github.com/shady2k/nocx/internal/session"
	"github.com/shady2k/nocx/internal/shellintegration"
)

type publishFailureParams struct {
	SessionID    string `json:"sessionId"`
	InstanceID   string `json:"instanceId"`
	SessionEpoch uint64 `json:"sessionEpoch"`
	Reason       string `json:"reason"`
}

// recordPublishFailure retains the fact and emits it to whoever is attached.
//
// It is called from the open path once the session exists and the ack has gone
// out (AD-7), so the notification always follows the id it addresses.
func (s *WSServer) recordPublishFailure(sessionID session.ID, reason shellintegration.PublishFailureReason) {
	if sessionID == "" || reason == shellintegration.PublishFailureNone {
		return
	}
	s.publishMu.Lock()
	if s.publishFailures == nil {
		s.publishFailures = make(map[session.ID]shellintegration.PublishFailureReason)
	}
	s.publishFailures[sessionID] = reason
	s.publishMu.Unlock()
	s.emitPublishFailure(sessionID)
}

func (s *WSServer) emitPublishFailure(sessionID session.ID) {
	s.publishMu.Lock()
	reason, ok := s.publishFailures[sessionID]
	s.publishMu.Unlock()
	if !ok {
		return
	}
	rx := s.getRx(sessionID)
	if rx == nil {
		return
	}
	wconn, _ := rx.getSubscriber()
	if wconn == nil {
		return
	}
	sess, err := s.registry.Get(sessionID)
	if err != nil {
		return
	}
	ident := sess.Identity()
	params := publishFailureParams{
		SessionID:    string(sessionID),
		InstanceID:   string(ident.InstanceID),
		SessionEpoch: ident.Epoch,
		Reason:       string(reason),
	}
	if err := wconn.TryNotify("session.publishFailed", mustMarshal(params)); err != nil {
		s.log.Debug("write session.publishFailed", "session", sessionID, "error", err)
	}
}

// replayPublishFailure re-sends the retained fact to a subscriber that attached
// after it settled, exactly as replayToolSurface does for its own.
func (s *WSServer) replayPublishFailure(sessionID session.ID) {
	s.emitPublishFailure(sessionID)
}
