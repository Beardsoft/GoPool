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

type RecorderOption func(*Recorder)

func WithConfigHash(hash string) RecorderOption { return func(r *Recorder) { r.configHash = hash } }

func NewRecorder(q *db.Queries, options ...RecorderOption) *Recorder {
	r := &Recorder{q: q}
	for _, option := range options {
		option(r)
	}
	return r
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
	}
	err := r.q.UpsertRuntimeStatus(ctx, params)
	return err
}

func (r *Recorder) RecordSnapshot(ctx context.Context, s Snapshot) error {
	if !r.lastSnapshot.IsZero() && time.Since(r.lastSnapshot) < time.Minute {
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

func (r *Recorder) Prune(ctx context.Context) error {
	// Prune implementation omitted for now; placeholder to satisfy interface
	return nil
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
