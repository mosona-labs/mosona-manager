ALTER TABLE agents
  ADD COLUMN IF NOT EXISTS protocol_version int2;

UPDATE agents
SET protocol_version = CASE
  WHEN btrim(public_key) <> '' THEN 2
  ELSE 1
END
WHERE protocol_version IS NULL;

ALTER TABLE agents
  ALTER COLUMN protocol_version SET DEFAULT 2,
  ALTER COLUMN protocol_version SET NOT NULL;

ALTER TABLE agents
  DROP CONSTRAINT IF EXISTS agents_protocol_version_valid;

ALTER TABLE agents
  ADD CONSTRAINT agents_protocol_version_valid
  CHECK (protocol_version IN (1, 2));

ALTER TABLE agents
  DROP CONSTRAINT IF EXISTS agents_legacy_identity_unpinned;

ALTER TABLE agents
  ADD CONSTRAINT agents_legacy_identity_unpinned
  CHECK (protocol_version <> 1 OR btrim(public_key) = '');
