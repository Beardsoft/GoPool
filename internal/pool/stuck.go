package pool

// isStuck reports whether a pending payout, submitted at block submittedHeight,
// has been waiting longer than stuckEpochs full epochs relative to head.
// A submittedHeight of 0 (unknown) never auto-fails.
func isStuck(submittedHeight int64, head uint32, stuckEpochs int, blocksPerEpoch uint32) bool {
	if submittedHeight <= 0 || stuckEpochs <= 0 || blocksPerEpoch == 0 {
		return false
	}
	threshold := int64(stuckEpochs) * int64(blocksPerEpoch)
	return int64(head)-submittedHeight > threshold
}
