UPDATE servers AS s
SET category = (
  SELECT c.id
  FROM categories AS c
  WHERE c.team = s.team_id
  ORDER BY c.sort, c.id
  LIMIT 1
)
WHERE NOT EXISTS (
  SELECT 1
  FROM categories AS current_category
  WHERE current_category.team = s.team_id
    AND current_category.id = s.category
);

ALTER TABLE servers
  DROP CONSTRAINT IF EXISTS "FK_SC";

ALTER TABLE categories
  DROP CONSTRAINT IF EXISTS categories_team_id_key;

ALTER TABLE categories
  ADD CONSTRAINT categories_team_id_key UNIQUE (team, id);

ALTER TABLE servers
  ADD CONSTRAINT "FK_SC"
  FOREIGN KEY (team_id, category) REFERENCES categories (team, id)
  ON DELETE RESTRICT ON UPDATE NO ACTION;
