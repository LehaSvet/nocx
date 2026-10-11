package transport

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/shady2k/nocx/internal/helper/proto"
	"github.com/shady2k/nocx/internal/lifecycle"
	"github.com/shady2k/nocx/internal/session"
)

type sessionEffectParams struct {
	SessionID      string   `json:"sessionId"`
	Generation     string   `json:"generation"`
	EffectID       string   `json:"effectId"`
	Kind           string   `json:"kind"`
	Title          string   `json:"title"`
	Body           string   `json:"body"`
	CommandNames   []string `json:"commandNames,omitempty"`
	EpisodeID      string   `json:"episodeId,omitempty"`
	ScreenRevision uint64   `json:"screenRevision,omitempty"`
}

func effectKindName(kind proto.EffectKind) (string, bool) {
	switch kind {
	case proto.EffectBell:
		return "bell", true
	case proto.EffectNotification:
		return "notification", true
	case proto.EffectClipboard:
		return "clipboard", true
	case proto.EffectTitle:
		return "title", true
	case proto.EffectCwdReport:
		return "cwd", true
	case proto.EffectPromptBoundary:
		return "promptBoundary", true
	case proto.EffectRecovery:
		return "recovery", true
	case proto.EffectCommandSnapshot:
		return "commandSnapshot", true
	default:
		return "", false
	}
}

const (
	maxCommandSnapshotNames     = 4096
	maxCommandSnapshotNameBytes = 65536
	maxCommandSnapshotBodyBytes = 1 << 20
)

func decodeCommandSnapshotBody(body []byte) ([]string, bool) {
	if len(body) == 0 || len(body) > maxCommandSnapshotBodyBytes || !utf8.Valid(body) {
		return nil, false
	}
	var names []string
	if err := json.Unmarshal(body, &names); err != nil || len(names) == 0 || len(names) > maxCommandSnapshotNames {
		return nil, false
	}
	total := 0
	for _, name := range names {
		if name == "" || !utf8.ValidString(name) {
			return nil, false
		}
		total += len(name)
		if total > maxCommandSnapshotNameBytes {
			return nil, false
		}
	}
	return names, true
}

// recordRecoverySighting records only the exact current lost episode. The
// runtime has already matched the private nonce; the lifecycle kernel stores
// only the public episode ID and sighted state. This runs before notification
// delivery so reconnect resync survives a dropped effect.
func (s *WSServer) recordRecoverySighting(sid session.ID, episodeID string) bool {
	if s.lifecyclePub == nil {
		return false
	}
	s.lifecycleMu.Lock()
	var lanes []lifecycle.LaneID
	for lane, owner := range s.lifecycleLanes {
		if owner == sid {
			lanes = append(lanes, lane)
		}
	}
	s.lifecycleMu.Unlock()
	for _, lane := range lanes {
		if err := s.lifecyclePub.SightRecovery(lane, episodeID); err == nil {
			s.recoveryMu.Lock()
			if rec := s.recoveries[sid]; rec != nil && rec.episodeID == episodeID {
				rec.mu.Lock()
				rec.sighted = true
				rec.mu.Unlock()
			}
			s.recoveryMu.Unlock()
			return true
		}
	}
	return false
}

// PublishSessionEffect sends one runtime effect to the current session owner.
// It is a control-plane event, never part of a replayable full screen frame.
func (s *WSServer) PublishSessionEffect(sid session.ID, effect proto.EffectFrame) bool {
	kind, ok := effectKindName(effect.Kind)
	if !ok || effect.EffectID == 0 || effect.Generation == 0 {
		return false
	}
	if effect.Kind == proto.EffectPromptBoundary {
		if effect.ScreenRevision == 0 {
			return false
		}
	} else if effect.ScreenRevision != 0 {
		return false
	}
	var commandNames []string
	if effect.Kind == proto.EffectRecovery {
		if len(effect.Title) != 0 || len(effect.Body) != 0 || effect.EpisodeID == "" || !s.recordRecoverySighting(sid, effect.EpisodeID) {
			return false
		}
	} else if effect.EpisodeID != "" {
		return false
	}
	if effect.Kind == proto.EffectCommandSnapshot {
		if len(effect.Title) != 0 || effect.ScreenRevision != 0 || effect.EpisodeID != "" {
			return false
		}
		var ok bool
		commandNames, ok = decodeCommandSnapshotBody(effect.Body)
		if !ok {
			return false
		}
	}
	rx := s.getRx(sid)
	if rx == nil {
		return false
	}
	wconn, _ := rx.getSubscriber()
	if wconn == nil {
		return false
	}
	body := string(effect.Body)
	if effect.Kind == proto.EffectCommandSnapshot {
		body = ""
	}
	params := sessionEffectParams{
		SessionID: string(sid), Generation: strconv.FormatUint(effect.Generation, 10),
		EffectID: strconv.FormatUint(effect.EffectID, 10), Kind: kind,
		Title: string(effect.Title), Body: body, CommandNames: commandNames, EpisodeID: effect.EpisodeID,
		ScreenRevision: effect.ScreenRevision,
	}
	waitForBytes := effect.Kind == proto.EffectPromptBoundary && effect.StreamOffset > 0
	offset := effect.StreamOffset
	accepted, coalesced := rx.queueEffect(wconn, offset, waitForBytes, effect.ScreenRevision, "session.effect", mustMarshal(params))
	if !accepted {
		s.log.Debug("session.effect dropped: pending delivery budget full or subscriber changed", "session", string(sid), "effect_id", effect.EffectID)
		return false
	}
	if coalesced {
		s.log.Debug("coalesced an older pending prompt boundary", "session", string(sid), "effect_id", effect.EffectID)
	}
	if effect.ScreenRevision > 0 {
		s.ensureScreenOffer(sid, rx, wconn, effect.ScreenRevision)
	}
	if cursor, ok := rx.deliveryCursor(wconn); ok {
		if !rx.enqueueDueEffects(wconn, cursor) {
			// The item remains at the head of the bounded queue. ringToConn
			// retries it through its cancellable outbound-room wait.
			s.log.Debug("session.effect queued behind outbound backpressure", "session", string(sid), "effect_id", effect.EffectID)
		}
	}
	return true
}
