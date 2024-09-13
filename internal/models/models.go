package models

type Validator struct {
	Address        string `json:"address"`
	SigningKey     string `json:"signingKey"`
	VotingKey      string `json:"votingKey"`
	RewardAddress  string `json:"rewardAddress"`
	Balance        int64  `json:"balance"`
	NumStakers     int64  `json:"numStakers"`
	InactivityFlag *int64 `json:"inactivityFlag"`
	Retired        bool   `json:"retired"`
	JailedFrom     *int64 `json:"jailedFrom"`
}

type Inherent struct {
	Type             string `json:"type"`
	BlockNumber      int64  `json:"blockNumber"`
	BlockTime        int64  `json:"blockTime"`
	ValidatorAddress string `json:"validatorAddress"`
	Target           string `json:"target"`
	Value            int64  `json:"value"`
	Hash             string `json:"hash"` // Add this field if it's present in the response
}

type Reward struct {
	Type             string `json:"type"`
	BlockNumber      int64  `json:"blockNumber"`
	BlockTime        int64  `json:"blockTime"`
	ValidatorAddress string `json:"validatorAddress"`
	Target           string `json:"target"`
	Value            int64  `json:"value"`
	Hash             string `json:"hash"`
}

func InherentToReward(inherent Inherent) Reward {
	return Reward{
		Type:             inherent.Type,
		BlockNumber:      inherent.BlockNumber,
		BlockTime:        inherent.BlockTime,
		ValidatorAddress: inherent.ValidatorAddress,
		Target:           inherent.Target,
		Value:            inherent.Value,
		Hash:             inherent.Hash,
	}
}

type PolicyConstants struct {
	StakingContractAddress    string `json:"stakingContractAddress"`
	CoinbaseAddress           string `json:"coinbaseAddress"`
	TransactionValidityWindow int64  `json:"transactionValidityWindow"`
	MaxSizeMicroBody          int64  `json:"maxSizeMicroBody"`
	Version                   int64  `json:"version"`
	Slots                     int64  `json:"slots"`
	BlocksPerBatch            int64  `json:"blocksPerBatch"`
	BatchesPerEpoch           int64  `json:"batchesPerEpoch"`
	BlocksPerEpoch            int64  `json:"blocksPerEpoch"`
	ValidatorDeposit          int64  `json:"validatorDeposit"`
	MinimumStake              int64  `json:"minimumStake"`
	TotalSupply               int64  `json:"totalSupply"`
	BlockSeparationTime       int64  `json:"blockSeparationTime"`
	JailEpochs                int64  `json:"jailEpochs"`
	GenesisBlockNumber        int64  `json:"genesisBlockNumber"`
}
