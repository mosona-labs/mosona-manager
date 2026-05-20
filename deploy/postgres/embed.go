package postgres

import "embed"

// InitSchema and Migrations are embedded into the app binary so prebuilt
// deployments do not need extra SQL files next to compose.yml.
//
//go:embed init/001_schema.sql
var InitSchema embed.FS

//go:embed migrations/*.sql
var Migrations embed.FS
