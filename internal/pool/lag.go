package pool

import (
	"context"
	"fmt"
	"time"

	"github.com/Beardsoft/GoPool/internal/notifier"
	"github.com/Beardsoft/GoPool/internal/ops"
)

const (
	lagWarnBlocks    int64 = 30
	lagRecoverBlocks int64 = 2
	lagEscalateAfter       = 2 * time.Minute
)

type lagAlertKind int

const (
	lagAlertNone lagAlertKind = iota
	lagAlertWarning
	lagAlertError
)

type lagAlertState struct {
	warningSent bool
	errorSent   bool
	since       time.Time
}

func chainLag(head uint32, processed int64) int64 {
	lag := int64(head) - processed
	if lag < 0 {
		return 0
	}
	return lag
}

// nextLagAlert is amber on first crossing of lagWarnBlocks, then bright-red
// error if the cursor has not recovered to lagRecoverBlocks within lagEscalateAfter.
func nextLagAlert(s lagAlertState, lag int64, now time.Time) (lagAlertState, lagAlertKind) {
	if lag <= lagRecoverBlocks {
		return lagAlertState{}, lagAlertNone
	}
	if !s.warningSent {
		if lag < lagWarnBlocks {
			return s, lagAlertNone
		}
		s.warningSent = true
		s.since = now
		return s, lagAlertWarning
	}
	if !s.errorSent && now.Sub(s.since) >= lagEscalateAfter {
		s.errorSent = true
		return s, lagAlertError
	}
	return s, lagAlertNone
}

func (m *Manager) maybeAlertChainLag(ctx context.Context, head uint32, processed int64) {
	now := time.Now()
	lag := chainLag(head, processed)
	var kind lagAlertKind
	m.lagAlert, kind = nextLagAlert(m.lagAlert, lag, now)
	if kind == lagAlertNone {
		return
	}

	alert := notifier.Alert{
		Level:   "warning",
		Type:    "chain_lag",
		Title:   "Chain lag",
		Message: fmt.Sprintf("Daemon is %d blocks behind head.", lag),
		Fields: []notifier.AlertField{
			{Name: "Lag", Value: fmt.Sprintf("%d blocks", lag)},
			{Name: "Processed", Value: fmt.Sprintf("%d", processed)},
			{Name: "Head", Value: fmt.Sprintf("%d", head)},
		},
	}
	severity := "warning"
	if kind == lagAlertError {
		alert.Level = "error"
		alert.Title = "Chain lag not recovering"
		alert.Message = fmt.Sprintf("Still %d blocks behind after %s. Check RPC and the daemon.", lag, lagEscalateAfter)
		severity = "error"
	}
	if m.notifier != nil {
		m.notifier.Send(ctx, alert)
	}
	if m.recorder != nil {
		_ = m.recorder.RecordEvent(ctx, ops.EventInput{
			Severity: severity,
			Category: "chain",
			Source:   "daemon",
			Type:     alert.Type,
			Summary:  alert.Title,
			Context:  map[string]any{"lag": lag, "processed": processed, "head": head},
		})
	}
}
