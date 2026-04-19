package db

import (
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
	"strings"
)

func GetTeamPublicPage(teamID int64) (_type.TeamPublicPage, error) {
	var page _type.TeamPublicPage
	err := Db.Get(
		&page,
		`SELECT team_id, enabled, name, domain, title, description, created_at, updated_at
		 FROM team_public_pages
		 WHERE team_id = $1`,
		teamID,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return _type.TeamPublicPage{
				TeamID:  teamID,
				Enabled: false,
			}, nil
		}
		return _type.TeamPublicPage{}, err
	}

	return page, nil
}

func UpsertTeamPublicPage(teamID int64, enabled bool, name, domain, title, description *string) error {
	_, err := Db.Exec(
		`INSERT INTO team_public_pages (team_id, enabled, name, domain, title, description, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, now())
		 ON CONFLICT (team_id)
		 DO UPDATE SET
		 	enabled = EXCLUDED.enabled,
		 	name = EXCLUDED.name,
		 	domain = EXCLUDED.domain,
		 	title = EXCLUDED.title,
		 	description = EXCLUDED.description,
		 	updated_at = now()`,
		teamID, enabled, name, domain, title, description,
	)
	return err
}

func GetEnabledTeamPublicPageByName(name string) (_type.ResolvedPublicPage, error) {
	var page _type.ResolvedPublicPage
	err := Db.Get(
		&page,
		`SELECT p.team_id, p.enabled, p.name, p.domain, p.title, p.description, p.created_at, p.updated_at,
		        t.name AS team_name, t.color AS team_color, t.image AS team_image
		 FROM team_public_pages p
		 JOIN teams t ON t.id = p.team_id
		 WHERE p.enabled = TRUE AND lower(p.name) = $1`,
		strings.ToLower(name),
	)
	return page, err
}

func GetEnabledTeamPublicPageByDomain(domain string) (_type.ResolvedPublicPage, error) {
	var page _type.ResolvedPublicPage
	err := Db.Get(
		&page,
		`SELECT p.team_id, p.enabled, p.name, p.domain, p.title, p.description, p.created_at, p.updated_at,
		        t.name AS team_name, t.color AS team_color, t.image AS team_image
		 FROM team_public_pages p
		 JOIN teams t ON t.id = p.team_id
		 WHERE p.enabled = TRUE AND lower(p.domain) = $1`,
		strings.ToLower(domain),
	)
	return page, err
}
