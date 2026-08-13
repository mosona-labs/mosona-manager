-- Every team owner must also be an administrator member of that team.
INSERT INTO m_team_user (team_id, user_id, role)
SELECT id, owner_id, 0
FROM teams
ON CONFLICT (team_id, user_id) DO UPDATE
SET role = 0
WHERE m_team_user.role <> 0;
