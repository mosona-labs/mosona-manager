package db

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestGetPassiveMonitoringAgentPublicKeyRequiresMonitoring(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`SELECT server_id, public_key FROM agents a JOIN servers s ON a.server_id = s.id WHERE s.type = 2 AND agent_uid = \$1 AND s.allow_monitor = true`).
		WithArgs("agent-uid").
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "public_key"}).AddRow(91, "public-key"))

	serverID, publicKey, err := GetPassiveMonitoringAgentPublicKey("agent-uid")
	if err != nil {
		t.Fatal(err)
	}
	if serverID != 91 || publicKey != "public-key" {
		t.Fatalf("lookup = (%d, %q), want (91, public-key)", serverID, publicKey)
	}
}

func TestGetPassiveAgentPublicKeyKeepsNonMonitoringRoutesAvailable(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`SELECT server_id, public_key FROM agents a JOIN servers s ON a.server_id = s.id WHERE s.type = 2 AND agent_uid = \$1$`).
		WithArgs("agent-uid").
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "public_key"}).AddRow(91, "public-key"))

	if _, _, err := GetPassiveAgentPublicKey("agent-uid"); err != nil {
		t.Fatal(err)
	}
}

func TestGetPassiveTerminalAgentPublicKeyRequiresTerminalAndInstalledAgent(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`SELECT server_id, public_key FROM agents a JOIN servers s ON a.server_id = s.id WHERE s.type = 2 AND agent_uid = \$1 AND s.allow_terminal = true AND a.status = 1`).
		WithArgs("agent-uid").
		WillReturnRows(sqlmock.NewRows([]string{"server_id", "public_key"}).AddRow(91, "public-key"))

	serverID, publicKey, err := GetPassiveTerminalAgentPublicKey("agent-uid")
	if err != nil {
		t.Fatal(err)
	}
	if serverID != 91 || publicKey != "public-key" {
		t.Fatalf("lookup = (%d, %q), want (91, public-key)", serverID, publicKey)
	}
}

func TestIsPassiveServerMonitoringEnabled(t *testing.T) {
	mock := setUserConfigMockDB(t)
	mock.ExpectQuery(`SELECT type = 2 AND allow_monitor FROM servers WHERE id = \$1`).
		WithArgs(int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"allow_monitor"}).AddRow(false))

	enabled, err := IsPassiveServerMonitoringEnabled(91)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("monitoring unexpectedly enabled")
	}
}
