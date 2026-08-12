package db

import (
	"strings"
	"testing"

	"mosona-manager/deploy/postgres"
)

func TestSecureOIDCIdentityMigrationQuarantinesLegacyZeroSubject(t *testing.T) {
	migration, err := postgres.Migrations.ReadFile("migrations/20260812_secure_oidc_identity.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(migration)
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS protocol",
		"ADD COLUMN IF NOT EXISTS issuer_url",
		"ADD COLUMN IF NOT EXISTS identity_namespace_version",
		"ADD COLUMN IF NOT EXISTS config_revision",
		"ADD COLUMN IF NOT EXISTS quarantined",
		"UPDATE auth_identity\nSET quarantined = true\nWHERE subject IN ('', '0')\n   OR subject ~ '^[[:space:]]|[[:space:]]$'",
		"auth_identity_quarantine_audit",
		"duplicate user/provider binding",
		"auth_identity_active_user_provider_unique",
		"AND subject !~ '^[[:space:]]|[[:space:]]$'",
		"CHECK (identity_namespace_version > 0)",
		"CHECK (config_revision > 0)",
	} {
		if !strings.Contains(sqlText, required) {
			t.Fatalf("migration does not contain %q", required)
		}
	}
	if strings.Contains(sqlText, "identity_id bigint PRIMARY KEY REFERENCES auth_identity") {
		t.Fatal("quarantine audit records must survive deletion of the quarantined identity")
	}
	if strings.Index(sqlText, "legacy invalid subject") > strings.Index(sqlText, "UPDATE auth_identity\nSET quarantined = true") {
		t.Fatal("legacy invalid subjects must be audited before they are quarantined")
	}
}
