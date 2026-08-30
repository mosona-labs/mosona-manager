DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_attribute
    WHERE attrelid = 'agents'::regclass
      AND attname = 'private_key'
      AND atttypid = 'text'::regtype
      AND NOT attisdropped
  ) THEN
    ALTER TABLE agents
      ALTER COLUMN private_key DROP DEFAULT;

    ALTER TABLE agents
      ALTER COLUMN private_key TYPE bytea
      USING convert_to(private_key, 'UTF8');

    ALTER TABLE agents
      ALTER COLUMN private_key SET DEFAULT ''::bytea;
  END IF;
END
$$;

LOCK TABLE agents IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM agents
    WHERE agent_uid <> ''
    GROUP BY agent_uid
    HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION 'cannot enforce unique Agent identities: duplicate non-empty agent_uid values exist'
      USING ERRCODE = '23505';
  END IF;
END
$$;

DROP INDEX IF EXISTS "IDX_AAU";

CREATE UNIQUE INDEX "IDX_AAU" ON agents USING btree (
  agent_uid COLLATE "pg_catalog"."default" "pg_catalog"."bpchar_ops" ASC NULLS LAST
) WHERE agent_uid <> '';
