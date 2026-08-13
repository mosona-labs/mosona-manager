ALTER TABLE ssh
  ADD COLUMN IF NOT EXISTS trust_legacy_host_key boolean NOT NULL DEFAULT false;

-- Preserve connectivity for SSH servers created before host-key pinning existed.
-- New rows inherit false and must complete explicit host-key confirmation.
UPDATE ssh
SET trust_legacy_host_key = true
WHERE host_key IS NULL;

ALTER TABLE ssh
  DROP CONSTRAINT IF EXISTS ssh_host_key_trust_state;

ALTER TABLE ssh
  ADD CONSTRAINT ssh_host_key_trust_state
  CHECK ((host_key IS NULL) = trust_legacy_host_key);
