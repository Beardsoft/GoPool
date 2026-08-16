// Package metrics exposes GoPool daemon Prometheus collectors and the
// scrape HTTP handler. Collectors live on the default registry.
package metrics

import (
	"context"
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ChainHead = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_chain_head",
		Help: "Current chain head height as seen by the daemon.",
	})
	LastProcessedHeight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_last_processed_height",
		Help: "DB cursor height last processed by the daemon.",
	})
	TickDuration = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_tick_duration_seconds",
		Help: "Wall time of the last daemon tick.",
	})
	Stakers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_stakers",
		Help: "Staker count from the current in_progress epoch snapshot.",
	})
	DelegatedStake = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_delegated_stake_luna",
		Help: "Delegated stake (luna) from the current in_progress epoch snapshot.",
	})
	PayslipsPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_payslips_pending",
		Help: "Payslips waiting to be paid.",
	})
	PayslipsPendingLuna = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_payslips_pending_luna",
		Help: "Sum of pending payslip amounts (luna).",
	})
	PayslipsStuck = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_payslips_stuck",
		Help: "Payslips in out_for_payment or awaiting_confirmation.",
	})
	LiveStake = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_live_stake_luna",
		Help: "Validator.Balance from the last GetValidator (luna).",
	})
	LiveStakers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gopool_live_stakers",
		Help: "Validator.NumStakers from the last GetValidator.",
	})
	ValidatorState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gopool_validator_state",
		Help: "Current validator state (1 for the active state, 0 otherwise).",
	}, []string{"state"})
	WalletBalance = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gopool_wallet_balance_luna",
		Help: "Liquid account balance (luna).",
	}, []string{"role"})
	RewardsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_rewards_luna_total",
		Help: "Cumulative reward inherents collected (luna).",
	})
	PoolFeeTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_pool_fee_luna_total",
		Help: "Cumulative pool fee withheld from rewards (luna).",
	})
	PayoutsSubmitted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gopool_payouts_submitted_total",
		Help: "Payout transactions submitted.",
	}, []string{"kind"})
	PayoutsConfirmed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_payouts_confirmed_total",
		Help: "Payout transactions confirmed.",
	})
	PayoutsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gopool_payouts_failed_total",
		Help: "Payout transactions that failed execution.",
	})
	PayoutLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "gopool_payout_latency_seconds",
		Help:    "Latency from audit log creation to payout submission.",
		Buckets: prometheus.ExponentialBuckets(60, 2, 15),
	})
	RPCErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gopool_rpc_errors_total",
		Help: "RPC failures in the daemon loop.",
	}, []string{"op"})
)

func init() {
	prometheus.MustRegister(
		ChainHead, LastProcessedHeight, TickDuration,
		Stakers, DelegatedStake,
		PayslipsPending, PayslipsPendingLuna, PayslipsStuck,
		LiveStake, LiveStakers, ValidatorState, WalletBalance,
		RewardsTotal, PoolFeeTotal,
		PayoutsSubmitted, PayoutsConfirmed, PayoutsFailed,
		PayoutLatency,
		RPCErrors,
	)
}

// SetValidatorState sets the 1/0 enum gauges. Unknown state leaves all at 0.
func SetValidatorState(state string) {
	for _, s := range []string{"active", "inactive", "jailed", "retired"} {
		v := 0.0
		if s == state {
			v = 1
		}
		ValidatorState.WithLabelValues(s).Set(v)
	}
}

// Handler serves GET /metrics and GET /healthz.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// Serve listens on addr until ctx is cancelled. Empty addr is a no-op.
func Serve(ctx context.Context, addr string) error {
	if addr == "" {
		return nil
	}
	srv := &http.Server{Addr: addr, Handler: Handler()}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
