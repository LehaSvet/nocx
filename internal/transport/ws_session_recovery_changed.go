package transport

import (
	"context"

	"github.com/shady2k/nocx/internal/content"
	"github.com/shady2k/nocx/internal/log"
	"github.com/shady2k/nocx/internal/session"
)

// session.recoveryChanged is session.recoveryStatus's own answer, PUSHED
// (nocx-zg3k3.14.24).
//
// WHY A PUSH AND NOT ANOTHER READ. The recording answers "what does this
// session's output no longer hold" for a stream that is still moving: the
// recorder appends the bytes the helper replays when a coordinator takes a
// session back, and the retention bound evicts the oldest bytes as it does.
// A reclaim asks ONCE, before it attaches, so its answer is a fact about an
// instant that has already passed by the time the card is drawn — and while a
// client stays attached the answer keeps changing and that client never hears
// about it. The card then states one number while `session.recoveryStatus`,
// asked a moment later, states another. Measured on PR #380's CI: the card
// said 3.1 MB, the same session's ledger said 3.6 MB, and no amount of
// waiting reconciled them.
//
// WHY NOT LET THE CLIENT RE-READ. A re-read has to be triggered by something,
// and the client's only observable is its own received offset. The recorder
// lags that offset — it is a second consumer of the same ring, appending in
// SQLite transactions — so the last evictions happen after the client has
// already drained, and a client that refreshed on catch-up would settle on a
// number that is permanently short. The recorder is the only party that knows
// when its own answer changed, which makes this notification the smallest
// thing that can be true.
//
// THE BODY IS THE RESULT, NOT A SECOND SHAPE OF IT. `sessionRecoveryStatusResult`
// is reused verbatim: a notification with its own field names would be the
// same fact spelled twice, and the two spellings would drift the first time
// either side gained a field. contracts/session.recoveryChanged.schema.json
// declares that shape for the wire, and the transport tests validate the same
// marshalled DTO against both schemas.
type sessionRecoveryRevision = sessionRecoveryStatusResult

// publishRecoveryRevision hands the recording's current answer to the
// session's attached pump, replacing any revision not yet delivered.
//
// REPLACING IS THE POINT. Every reader of this fact wants the newest one: a
// queue of superseded snapshots would make a person watch a number walk
// through its own history, and would let a backlog of notifications become
// the thing that keeps the client behind. The pump is woken rather than
// written to, so the RECORDER never waits on a client's socket — a notification
// admitted inline would stall this loop when the renderer is slow, and the ring
// cannot free bytes past a stalled recorder. The pump already blocks for room
// as ordinary output backpressure, which is where waiting belongs.
func (s *WSServer) publishRecoveryRevision(ctx context.Context, sid session.ID) {
	if s.sessionRecorder == nil {
		return
	}
	rx := s.getRx(sid)
	if rx == nil {
		return
	}
	wconn, _ := rx.getSubscriber()
	if wconn == nil {
		// Nobody is attached: there is no card to revise, and the next client
		// to attach asks for itself (ipc.ts reclaimSession).
		return
	}
	status, err := s.sessionRecorder.RecoveryStatus(ctx, string(sid))
	if err != nil {
		log.From(ctx).Warn("the recording's revised recovery metadata could not be read",
			"session_id", string(sid), "error", err)
		return
	}
	if status.Gaps == nil {
		status.Gaps = []content.Gap{}
	}
	if msg := unsafeRecoveryStatus(status); msg != "" {
		log.From(ctx).Error("session recovery revision exceeds the JSON-safe byte-offset boundary",
			"session_id", string(sid), "error", msg)
		return
	}
	rx.publishRecovery(wconn, mustMarshal(sessionRecoveryRevision{
		SessionID: string(sid), Produced: status.Produced, Gaps: status.Gaps,
	}))
}
