ALTER TABLE transactions ADD COLUMN submitted_height INTEGER NOT NULL DEFAULT 0;

-- Backfill legacy rows (pre-migration) with the current chain head so they age
-- from "now" instead of being treated as infinitely old. COALESCE keeps the
-- NOT NULL column safe when the runtime_status row is not yet recorded (head
-- unknown -> 0, which the detector treats as "unknown" and never auto-fails).
UPDATE transactions
SET submitted_height = COALESCE((SELECT chain_head FROM runtime_status WHERE id = 1), 0)
WHERE submitted_height = 0;
