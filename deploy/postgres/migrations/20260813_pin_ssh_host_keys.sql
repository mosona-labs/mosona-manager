ALTER TABLE ssh
  ADD COLUMN IF NOT EXISTS host_key text;

ALTER TABLE ssh
  DROP CONSTRAINT IF EXISTS ssh_host_key_not_blank;

ALTER TABLE ssh
  ADD CONSTRAINT ssh_host_key_not_blank
  CHECK (host_key IS NULL OR btrim(host_key) <> '');
