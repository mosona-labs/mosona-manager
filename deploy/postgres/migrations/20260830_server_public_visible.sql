DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_attribute
    WHERE attrelid = 'servers'::regclass
      AND attname = 'public_visible'
      AND NOT attisdropped
  ) THEN
    ALTER TABLE servers ADD COLUMN public_visible bool NOT NULL DEFAULT true;
  END IF;
END
$$;
