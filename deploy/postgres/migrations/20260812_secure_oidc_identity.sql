ALTER TABLE auth_provider
  ADD COLUMN IF NOT EXISTS protocol varchar(16) NOT NULL DEFAULT 'oauth2',
  ADD COLUMN IF NOT EXISTS issuer_url varchar(512) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS scopes text NOT NULL DEFAULT 'read:user read:email',
  ADD COLUMN IF NOT EXISTS subject_field varchar(255) NOT NULL DEFAULT 'id',
  ADD COLUMN IF NOT EXISTS identity_namespace_version bigint NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS config_revision bigint NOT NULL DEFAULT 1;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'auth_provider_protocol_check'
      AND conrelid = 'auth_provider'::regclass
  ) THEN
    ALTER TABLE auth_provider
      ADD CONSTRAINT auth_provider_protocol_check
      CHECK (protocol IN ('oauth2', 'oidc'));
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'auth_provider_config_revision_check'
      AND conrelid = 'auth_provider'::regclass
  ) THEN
    ALTER TABLE auth_provider
      ADD CONSTRAINT auth_provider_config_revision_check
      CHECK (config_revision > 0);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'auth_provider_identity_namespace_version_check'
      AND conrelid = 'auth_provider'::regclass
  ) THEN
    ALTER TABLE auth_provider
      ADD CONSTRAINT auth_provider_identity_namespace_version_check
      CHECK (identity_namespace_version > 0);
  END IF;
END $$;

ALTER TABLE auth_identity
  ADD COLUMN IF NOT EXISTS quarantined boolean NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS auth_identity_quarantine_audit (
  identity_id bigint PRIMARY KEY,
  provider_id integer NOT NULL,
  user_id bigint NOT NULL,
  subject varchar(255) NOT NULL,
  reason text NOT NULL,
  quarantined_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO auth_identity_quarantine_audit (identity_id, provider_id, user_id, subject, reason)
SELECT id, provider_id, user_id, subject, 'legacy invalid subject'
FROM auth_identity
WHERE subject IN ('', '0')
   OR subject ~ '^[[:space:]]|[[:space:]]$'
ON CONFLICT (identity_id) DO NOTHING;

UPDATE auth_identity
SET quarantined = true
WHERE subject IN ('', '0')
   OR subject ~ '^[[:space:]]|[[:space:]]$';

WITH duplicate_bindings AS (
  SELECT id, provider_id, user_id, subject,
         row_number() OVER (PARTITION BY user_id, provider_id ORDER BY id) AS binding_rank
  FROM auth_identity
  WHERE quarantined = false
)
INSERT INTO auth_identity_quarantine_audit (identity_id, provider_id, user_id, subject, reason)
SELECT id, provider_id, user_id, subject, 'duplicate user/provider binding'
FROM duplicate_bindings
WHERE binding_rank > 1
ON CONFLICT (identity_id) DO NOTHING;

WITH duplicate_bindings AS (
  SELECT id,
         row_number() OVER (PARTITION BY user_id, provider_id ORDER BY id) AS binding_rank
  FROM auth_identity
  WHERE quarantined = false
)
UPDATE auth_identity AS identity
SET quarantined = true
FROM duplicate_bindings
WHERE identity.id = duplicate_bindings.id
  AND duplicate_bindings.binding_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS auth_identity_active_user_provider_unique
  ON auth_identity (user_id, provider_id)
  WHERE quarantined = false;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'auth_identity_subject_check'
      AND conrelid = 'auth_identity'::regclass
  ) THEN
    ALTER TABLE auth_identity
      ADD CONSTRAINT auth_identity_subject_check
      CHECK (
        quarantined OR (
          subject <> ''
          AND subject <> '0'
          AND subject !~ '^[[:space:]]|[[:space:]]$'
        )
      );
  END IF;
END $$;
