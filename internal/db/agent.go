package db

import "fmt"

type passiveAgentAccess uint8

const (
	passiveAgentAccessGeneral passiveAgentAccess = iota
	passiveAgentAccessMonitoring
	passiveAgentAccessTerminal
)

func GetEnrollToken(hash string) (int64, error) {
	var serverId int64
	err := Db.Get(&serverId, "SELECT server_id FROM enroll_tokens WHERE token_hash = $1 AND NOT is_revoked", hash)
	return serverId, err
}

func GetPassiveAgentPublicKey(agentUID string) (int64, string, error) {
	return getPassiveAgentPublicKey(agentUID, passiveAgentAccessGeneral)
}

func GetPassiveMonitoringAgentPublicKey(agentUID string) (int64, string, error) {
	return getPassiveAgentPublicKey(agentUID, passiveAgentAccessMonitoring)
}

func GetPassiveTerminalAgentPublicKey(agentUID string) (int64, string, error) {
	return getPassiveAgentPublicKey(agentUID, passiveAgentAccessTerminal)
}

func getPassiveAgentPublicKey(agentUID string, access passiveAgentAccess) (int64, string, error) {
	var serverId int64
	var publicKey string
	query := "SELECT server_id, public_key FROM agents a JOIN servers s ON a.server_id = s.id WHERE s.type = 2 AND agent_uid = $1"
	switch access {
	case passiveAgentAccessGeneral:
	case passiveAgentAccessMonitoring:
		query += " AND s.allow_monitor = true"
	case passiveAgentAccessTerminal:
		// status=1 means enrollment completed. MainGet is the online-state gate.
		query += " AND s.allow_terminal = true AND a.status = 1"
	default:
		return 0, "", fmt.Errorf("invalid passive agent access mode: %d", access)
	}
	err := Db.QueryRow(query, agentUID).Scan(&serverId, &publicKey)
	return serverId, publicKey, err
}
