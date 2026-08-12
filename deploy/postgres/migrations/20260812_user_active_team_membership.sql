DELETE FROM users_config AS uc
WHERE NOT EXISTS (
  SELECT 1
  FROM m_team_user AS mtu
  WHERE mtu.team_id = uc.active_team
    AND mtu.user_id = uc.uid
);

ALTER TABLE users_config
  DROP CONSTRAINT IF EXISTS "FK_UCT";

ALTER TABLE users_config
  ADD CONSTRAINT "FK_UCT"
  FOREIGN KEY (active_team, uid)
  REFERENCES m_team_user (team_id, user_id)
  ON DELETE CASCADE
  ON UPDATE NO ACTION;
