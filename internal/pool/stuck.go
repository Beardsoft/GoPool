package pool

import "time"

// IsStuck reports whether a pending payout has been unconfirmable for longer
// than stuckEpochs full epochs.
//
// When the submission height is known it compares against the current head
// (height-based, exact). When the height was never recorded (0) it falls back
// to the transaction's wall-clock age, estimated from the chain's block
// separation time — this catches txs submitted before height tracking existed
// or whose height was lost. A payout with neither a height nor a timestamp is
// never treated as stuck.
func IsStuck(submittedHeight int64, head uint32, stuckEpochs int, blocksPerEpoch, blockSeparationMs uint32, submittedAt, now time.Time) bool {
	if stuckEpochs <= 0 {
		return false
	}
	if submittedHeight > 0 && blocksPerEpoch > 0 {
		threshold := int64(stuckEpochs) * int64(blocksPerEpoch)
		return int64(head)-submittedHeight > threshold
	}
	if !submittedAt.IsZero() && blocksPerEpoch > 0 && blockSeparationMs > 0 {
		epochMs := int64(blocksPerEpoch) * int64(blockSeparationMs)
		threshold := time.Duration(int64(stuckEpochs)*epochMs) * time.Millisecond
		return now.Sub(submittedAt) > threshold
	}
	return false
}
