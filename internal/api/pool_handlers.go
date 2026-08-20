package api

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/Beardsoft/GoPool/internal/db"
	"github.com/Beardsoft/GoPool/internal/logger"
	"github.com/Beardsoft/GoPool/internal/pool"

	"go.uber.org/zap"
)

type poolResponse struct {
	CurrentEpoch      int64           `json:"current_epoch"`
	EpochStatus       string          `json:"epoch_status"`
	NumStakers        int64           `json:"num_stakers"`
	TotalStakeLuna    int64           `json:"total_stake_luna"`
	TotalRewardsLuna  int64           `json:"total_rewards_luna"`
	PoolFeePercentage float64         `json:"pool_fee_percentage"`
	PoolName          string          `json:"pool_name"`
	PoolDescription   string          `json:"pool_description"`
	ContactURL        string          `json:"contact_url"`
	TelegramURL       string          `json:"telegram_url"`
	DiscordURL        string          `json:"discord_url"`
	XURL              string          `json:"x_url"`
	Disclosure        string          `json:"disclosure"`
	Network           string          `json:"network"`
	EpochClock        *epochClockJSON `json:"epoch_clock,omitempty"`
}

type epochClockJSON struct {
	Epoch             uint32 `json:"epoch"`
	Head              uint32 `json:"head"`
	BlocksIntoEpoch   uint32 `json:"blocks_into_epoch"`
	BlocksPerEpoch    uint32 `json:"blocks_per_epoch"`
	BlockSeparationMs uint32 `json:"block_separation_ms"`
	RemainingBlocks   uint32 `json:"remaining_blocks"`
	RemainingMs       uint64 `json:"remaining_ms"`
}

type rewardPointResponse struct {
	EpochNumber    int64 `json:"epoch_number"`
	TotalAmount    int64 `json:"total_amount"`
	TotalFee       int64 `json:"total_fee"`
	Batches        int64 `json:"batches"`
	NumStakers     int64 `json:"num_stakers"`
	TotalStakeLuna int64 `json:"total_stake_luna"`
}

func (a *API) handlePool(w http.ResponseWriter, r *http.Request) {
	epoch, err := a.queries.GetLatestEpoch(r.Context())
	if errors.Is(err, sql.ErrNoRows) {
		// Fresh pool: no epochs recorded yet. Report an empty state instead of
		// an error so the dashboard renders before the first election.
		epoch = db.GetLatestEpochRow{Number: 0, Status: "pending"}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool state")
		return
	}
	totalRewards, err := a.queries.SumRewardsAmount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool state")
		return
	}
	feePct := 0.0
	poolName := "GoPool"
	poolDescription, contactURL, telegramURL, discordURL, xURL, disclosure, network := "", "", "", "", "", "", ""
	if a.cfg != nil {
		feePct = a.cfg.PoolFeePercentage
		if a.cfg.PoolName != "" {
			poolName = a.cfg.PoolName
		}
		poolDescription, contactURL, telegramURL, discordURL, xURL, disclosure = a.cfg.PoolDescription, a.cfg.ContactURL, a.cfg.TelegramURL, a.cfg.DiscordURL, a.cfg.XURL, a.cfg.Disclosure
		network = a.cfg.Network
	}
	writeJSON(w, http.StatusOK, poolResponse{
		CurrentEpoch:      epoch.Number,
		EpochStatus:       epoch.Status,
		NumStakers:        epoch.NumStakers,
		TotalStakeLuna:    epoch.Balance,
		TotalRewardsLuna:  totalRewards,
		PoolFeePercentage: feePct,
		PoolName:          poolName, PoolDescription: poolDescription, ContactURL: contactURL,
		TelegramURL: telegramURL, DiscordURL: discordURL, XURL: xURL, Disclosure: disclosure,
		Network: network, EpochClock: a.epochClock(r),
	})
}

func (a *API) epochClock(r *http.Request) *epochClockJSON {
	if a.rpc == nil {
		return nil
	}
	ctx := r.Context()
	head, err := a.rpc.BlockNumber(ctx)
	if err != nil {
		logger.Logger.Warn("epoch clock: block number", zap.Error(err))
		return nil
	}
	policy, err := a.rpc.GetPolicy(ctx)
	if err != nil || policy == nil {
		logger.Logger.Warn("epoch clock: policy", zap.Error(err))
		return nil
	}
	clock := pool.EpochClockAt(policy, head)
	return &epochClockJSON{
		Epoch:             clock.Epoch,
		Head:              clock.Head,
		BlocksIntoEpoch:   clock.BlocksIntoEpoch,
		BlocksPerEpoch:    clock.BlocksPerEpoch,
		BlockSeparationMs: clock.BlockSeparationMs,
		RemainingBlocks:   clock.RemainingBlocks,
		RemainingMs:       clock.RemainingMs,
	}
}

func (a *API) handlePoolRewards(w http.ResponseWriter, r *http.Request) {
	rows, err := a.queries.ListRewardsByEpochRange(r.Context(), db.ListRewardsByEpochRangeParams{
		FromEpochNumber: 0,
		ToEpochNumber:   int64(^uint64(0) >> 1),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "loading pool rewards")
		return
	}

	points := make([]rewardPointResponse, len(rows))
	for i, row := range rows {
		points[i] = rewardPointResponse{
			EpochNumber:    row.EpochNumber,
			TotalAmount:    int64(row.TotalAmount.Float64),
			TotalFee:       int64(row.TotalFee.Float64),
			Batches:        row.Batches,
			NumStakers:     row.NumStakers,
			TotalStakeLuna: row.TotalStakeLuna,
		}
	}
	writeJSON(w, http.StatusOK, points)
}
