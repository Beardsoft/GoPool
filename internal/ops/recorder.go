package ops

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/Beardsoft/GoPool/internal/db"
)

type Recorder struct {
	q            *db.Queries
	lastSnapshot time.Time
	configHash   string
}

func NewRecorder(q *db.Queries, configHash string) *Recorder {
	return &Recorder{q: q, configHash: configHash}
}

func (r *Recorder) RecordEvent(ctx context.Context, in EventInput) error {
	ctxJSON, err := redactAndMarshal(in.Context)
	if err != nil {
		return err
	}
	params := db.InsertOperatorEventParams{
		Severity:      in.Severity,
		Category:      in.Category,
		Source:        in.Source,
		EventType:     in.Type,
		Summary:       in.Summary,
		ContextJson:   sqlNullString(ctxJSON),
		ActorAddress:  sqlNullString(in.ActorAddress),
		CorrelationID: sqlNullString(in.CorrelationID),
	}
	_, err = r.q.InsertOperatorEvent(ctx, params)
	return err
}

func (r *Recorder) RecordHeartbeat(ctx context.Context, hb Heartbeat) error {
	if hb.ConfigHash == "" {
		hb.ConfigHash = r.configHash
	}
	rpcOk := int64(0)
	if hb.RPCOk {
		rpcOk = 1
	}
	params := db.UpsertRuntimeStatusParams{
		HeartbeatAt:             hb.HeartbeatAt,
		DaemonVersion:           hb.DaemonVersion,
		ConfigHash:              hb.ConfigHash,
		DerivedValidatorAddress: hb.DerivedValidatorAddress,
		ValidatorState:          hb.ValidatorState,
		LastProcessedHeight:     hb.LastProcessedHeight,
		ChainHead:               hb.ChainHead,
		LastTickMs:              hb.LastTickMs,
		RpcOk:                   rpcOk,
		ReadinessError:          sqlNullString(hb.ReadinessError),
		EpochNumber:             sqlNullInt64(hb.EpochNumber, hb.HasEpochParticipation),
		EpochElected:            sqlNullInt64(boolInt(hb.EpochElected), hb.HasEpochParticipation),
		SlotCount:               sqlNullInt64(hb.SlotCount, hb.HasEpochParticipation),
		SlotsTotal:              sqlNullInt64(hb.SlotsTotal, hb.HasEpochParticipation),
	}
	err := r.q.UpsertRuntimeStatus(ctx, params)
	return err
}

// SnapshotDue reports whether a minute has passed since the last recorded
// snapshot (or none yet). Callers check it before doing the reads that feed
// RecordSnapshot, so they skip the work instead of doing it and discarding it.
func (r *Recorder) SnapshotDue() bool {
	return r.lastSnapshot.IsZero() || time.Since(r.lastSnapshot) >= time.Minute
}

func (r *Recorder) RecordSnapshot(ctx context.Context, s Snapshot) error {
	if !r.SnapshotDue() {
		return nil
	}
	rpcOk := int64(0)
	if s.RPCOk {
		rpcOk = 1
	}
	params := db.InsertHealthSnapshotParams{
		RecordedAt:         s.RecordedAt,
		ChainHead:          s.ChainHead,
		ProcessedHeight:    s.ProcessedHeight,
		TickMs:             s.TickMs,
		ValidatorState:     s.ValidatorState,
		LiveStake:          s.LiveStake,
		StakerCount:        s.StakerCount,
		PendingPayoutCount: s.PendingPayoutCount,
		PendingPayoutLuna:  s.PendingPayoutLuna,
		StuckPayoutCount:   s.StuckPayoutCount,
		StuckPayoutLuna:    s.StuckPayoutLuna,
		WalletBalance:      s.WalletBalance,
		RpcOk:              rpcOk,
	}
	_, err := r.q.InsertHealthSnapshot(ctx, params)
	if err == nil {
		r.lastSnapshot = s.RecordedAt
	}
	return err
}

func redactAndMarshal(v any) (string, error) {
	redacted := redactValue(v)
	b, err := json.Marshal(redacted)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func redactValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if shouldRedactKey(k) {
				out[k] = "[redacted]"
			} else {
				out[k] = redactValue(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = redactValue(val)
		}
		return out
	default:
		return v
	}
}

func shouldRedactKey(k string) bool {
	l := strings.ToLower(k)
	return l == "private_key" || l == "session_secret" || l == "token" || l == "authorization" || l == "cookie"
}

func sqlNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func sqlNullInt64(v int64, valid bool) sql.NullInt64 {
	if !valid {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: v, Valid: true}
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
