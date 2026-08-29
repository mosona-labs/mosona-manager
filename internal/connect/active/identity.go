package active

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/pkg/identity"
)

var (
	// ErrAgentIdentityUnpaired indicates that a non-pairing connection has no pinned Agent identity.
	ErrAgentIdentityUnpaired = errors.New("active agent identity is not paired")
	// ErrAgentIdentityMismatch indicates that an Agent identity differs from the pinned identity.
	ErrAgentIdentityMismatch = errors.New("active agent identity does not match the pinned key")
	// ErrLegacyAgentRequiresUpgrade indicates that compatibility connectivity succeeded but v2 pairing did not.
	ErrLegacyAgentRequiresUpgrade = errors.New("legacy Active Agent is online and still requires a v2 upgrade")
)

func pinActiveAgentPublicKey(serverID int64, candidate string) (string, error) {
	result, err := db.Db.Exec(
		"UPDATE agents SET public_key = $1, protocol_version = 2 WHERE server_id = $2 AND public_key = '' AND EXISTS (SELECT 1 FROM servers WHERE id = $2 AND type = 1)",
		candidate, serverID,
	)
	if err != nil {
		return "", err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if affected == 1 {
		recordActiveIdentityEvent(serverID, candidate, "Pinned first Active Agent identity", "high")
		return candidate, nil
	}
	var pinned string
	if err = db.Db.QueryRow("SELECT public_key FROM agents WHERE server_id = $1", serverID).Scan(&pinned); err != nil {
		return "", err
	}
	if pinned != candidate {
		return "", fmt.Errorf("%w: concurrent pairing selected another key", ErrAgentIdentityMismatch)
	}
	return pinned, nil
}

func markActiveAgentPaired(serverID int64) error {
	_, err := db.Db.Exec("UPDATE agents SET status = 1, last_seen_at = NOW() WHERE server_id = $1", serverID)
	return err
}

func recordActiveIdentityEvent(serverID int64, publicKeyPEM, message, level string) {
	publicKey, err := identity.ParseEd25519PublicKeyPEM([]byte(publicKeyPEM))
	if err != nil {
		log.Printf("Failed to record Active Agent identity event for Server (ID%d): parse public key: %v", serverID, err)
		return
	}
	fingerprint, err := identity.Ed25519Fingerprint(ed25519.PublicKey(publicKey))
	if err != nil {
		log.Printf("Failed to record Active Agent identity event for Server (ID%d): fingerprint public key: %v", serverID, err)
		return
	}
	var teamID int64
	if err = db.Db.QueryRow("SELECT team_id FROM servers WHERE id = $1", serverID).Scan(&teamID); err != nil {
		log.Printf("Failed to record Active Agent identity event for Server (ID%d): look up team: %v", serverID, err)
		return
	}
	influx.LogAdd(teamID, 0, "security", fmt.Sprintf("%s for Server (ID%d): %s", message, serverID, fingerprint), "", "mosona-manager-hub", level)
}
