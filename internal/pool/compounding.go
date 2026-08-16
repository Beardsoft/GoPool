package pool

// effectivePayoutKind resolves the payout tx kind for one staker. A stored
// preference (pref != nil) overrides the global mode; a nil pref falls back to
// the global mode. "delegate" only restakes while the staker is still delegated
// to this validator (choosePayoutTx), otherwise it degrades to a transfer.
func effectivePayoutKind(globalMode string, pref *bool, stillDelegated bool) payoutKind {
	mode := globalMode
	if pref != nil {
		if *pref {
			mode = "delegate"
		} else {
			mode = "transfer"
		}
	}
	return choosePayoutTx(mode, stillDelegated)
}
