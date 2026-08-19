package pool

import (
	"context"
	"fmt"

	"github.com/NimMiniApps/nimiq-go/rpc"

	"github.com/Beardsoft/GoPool/internal/metrics"
	"github.com/Beardsoft/GoPool/internal/notifier"
)

type epochParticipation struct {
	valid      bool
	epoch      uint32
	elected    bool
	slotCount  uint32
	slotsTotal uint32
}

type policySlots struct {
	Slots uint32 `json:"slots"`
}

func applyActiveSet(dst *epochParticipation, addr string, vs []rpc.Validator, epoch, slots uint32) {
	elected, count := slotShare(addr, vs, slots)
	*dst = epochParticipation{
		valid:      true,
		epoch:      epoch,
		elected:    elected,
		slotCount:  count,
		slotsTotal: slots,
	}
}

func (m *Manager) refreshEpochParticipation(ctx context.Context, head uint32) {
	if m.chain == nil || m.policy == nil {
		return
	}
	if m.slotsTotal == 0 {
		var policy policySlots
		if err := m.chain.RPC.Call(ctx, "getPolicyConstants", nil, &policy); err != nil {
			metrics.RPCErrors.WithLabelValues("policy").Inc()
			return
		}
		if policy.Slots == 0 {
			return
		}
		m.slotsTotal = policy.Slots
	}
	vs, err := m.chain.RPC.GetActiveValidators(ctx)
	if err != nil {
		metrics.RPCErrors.WithLabelValues("active_validators").Inc()
		return
	}
	applyActiveSet(&m.epochPart, m.chain.Address().String(), vs, epochAt(m.policy, head), m.slotsTotal)
	elected := 0.0
	if m.epochPart.elected {
		elected = 1
	}
	metrics.EpochElected.Set(elected)
	metrics.ValidatorSlots.Set(float64(m.epochPart.slotCount))
	metrics.SlotsTotal.Set(float64(m.epochPart.slotsTotal))
	m.maybeAlertNotElected(ctx)
}

func shouldAlertNotElected(alerted bool, alertedEpoch uint32, part epochParticipation) bool {
	if !part.valid || part.elected {
		return false
	}
	return !alerted || alertedEpoch != part.epoch
}

func (m *Manager) maybeAlertNotElected(ctx context.Context) {
	if !shouldAlertNotElected(m.unelectedAlerted, m.unelectedAlertEpoch, m.epochPart) {
		return
	}
	m.unelectedAlerted = true
	m.unelectedAlertEpoch = m.epochPart.epoch
	if m.notifier == nil {
		return
	}
	m.notifier.Send(ctx, notifier.Alert{
		Level:   "warning",
		Type:    "not_elected",
		Title:   "Not elected",
		Message: fmt.Sprintf("Validator was not elected for epoch %d (0 of %d slots).", m.epochPart.epoch, m.epochPart.slotsTotal),
		Fields: []notifier.AlertField{
			{Name: "Epoch", Value: fmt.Sprintf("%d", m.epochPart.epoch)},
			{Name: "Slots", Value: fmt.Sprintf("%d / %d", m.epochPart.slotCount, m.epochPart.slotsTotal)},
		},
	})
}
