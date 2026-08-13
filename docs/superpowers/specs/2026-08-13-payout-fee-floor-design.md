# Payout fee floor

## Context

Payout transactions currently hardcode fee `0` and send as soon as a
staker's pending payslips reach `min_payout_luna`. That works while the
network asks for nothing. When `minFeePerByte` is non-zero, those
transactions are rejected, and a payout can cost more in fees than it
delivers.

The pool fee (the cut that never gets a payslip) exists to cover operating
costs, including tx fees. Stakers should still receive their full share.

## Behavior

`min_payout_luna` remains the first gate (`GetEligibleForPayout`).

For each eligible staker:

1. Choose payout kind (delegate vs transfer) as today.
2. Build the unsigned transaction and call `rpc.Client.EstimateFee`.
3. If `fee > 0` and `amount < fee * 10`, leave the payslips `pending` and
   skip this staker until a later tick. More batches will accumulate.
4. Otherwise set `tx.Fee` to the estimate, mark payslips
   `out_for_payment`, sign, and submit. The staker is paid `amount` in
   full; the pool pays the fee on top.

When `fee == 0`, the 10× rule is a no-op. Today's network behavior is
unchanged.

The 10× multiple is a fixed constant, not a config field.

## Payout loop

Fee check happens **before** marking `out_for_payment`, so a hold cannot
stick slips in a non-pending state.

Order: choose kind → build unsigned tx → `EstimateFee` →
`payoutWorthSending` (skip or continue) → mark → sign/send with `tx.Fee`
set.

If submit fails after the mark, reset that address's `out_for_payment`
payslips back to `pending` so the next tick retries. Today a failed
submit can leave slips stuck.

Skip-for-fee logs at debug, not info: the daemon ticks every ~2s and
info-level skips would spam.

## Helper

```
payoutWorthSending(amount, fee) bool
```

- `fee == 0` → true (SQL already applied `min_payout_luna`)
- otherwise → `amount >= fee * 10`

## Out of scope

- Protocol `minimum_stake` (compounding an existing staker has no extra
  minimum; undelegated stakers get a basic transfer)
- Configurable fee ratio
- Reward-address vs payout-wallet split
- Validator deactivate execution failures
