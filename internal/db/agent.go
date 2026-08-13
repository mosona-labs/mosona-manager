package db

func GetEnrollToken(hash string) (int64, error) {
	var serverId int64
	err := Db.Get(&serverId, "SELECT server_id FROM enroll_tokens WHERE token_hash = $1 AND NOT is_revoked", hash)
	return serverId, err
}

func GetPassiveAgentPublicKey(agentUID string) (int64, string, error) {
	return getPassiveAgentPublicKey(agentUID, false)
}

func GetPassiveMonitoringAgentPublicKey(agentUID string) (int64, string, error) {
	return getPassiveAgentPublicKey(agentUID, true)
}

func getPassiveAgentPublicKey(agentUID string, requireMonitoring bool) (int64, string, error) {
	var serverId int64
	var publicKey string
	query := "SELECT server_id, public_key FROM agents a JOIN servers s ON a.server_id = s.id WHERE s.type = 2 AND agent_uid = $1"
	if requireMonitoring {
		query += " AND s.allow_monitor = true"
	}
	err := Db.QueryRow(query, agentUID).Scan(&serverId, &publicKey)
	return serverId, publicKey, err
}
