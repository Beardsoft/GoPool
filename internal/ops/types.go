package ops

import "time"

type EventInput struct {
	Severity     string
	Category     string
	Source       string
	Type         string
	Summary      string
	Context      map[string]any
	ActorAddress string
	CorrelationID string
}

type Heartbeat struct {
	HeartbeatAt              time.Time
	DaemonVersion            string
	ConfigHash               string
	DerivedValidatorAddress  string
	ValidatorState           string
	LastProcessedHeight      int64
	ChainHead                int64
	LastTickMs               int64
	RPCOk                    bool
	ReadinessError           string
}

type Snapshot struct {
	RecordedAt           time.Time
	ChainHead            int64
	ProcessedHeight      int64
	TickMs               int64
	ValidatorState       string
	LiveStake            int64
	StakerCount          int64
	PendingPayoutCount   int64
	PendingPayoutLuna    int64
	StuckPayoutCount     int64
	StuckPayoutLuna      int64
	WalletBalance        int64
	RPCOk                bool
}
