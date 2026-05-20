package postgres

import "embed"

// Migrations is embedded into the app binary so prebuilt deployments can run
// schema upgrades during normal application startup.
//
//go:embed migrations/*.sql
var Migrations embed.FS
