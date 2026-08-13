package db

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils/encrypt"

	"github.com/Masterminds/squirrel"
)

func ListTerminals(teamId int64) ([]_type.Terminal, error) {
	var servers = make([]_type.Terminal, 0)

	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

	query := psql.Select(
		"s.id", "s.type", "s.name", "s.weight", "s.category",
		"i.os",
		"COALESCE(ssh.username, NULL)",
		"COALESCE(ssh.address, NULL)",
		"COALESCE(ssh.port, NULL)",
	).
		From("servers s").
		LeftJoin("server_info i ON s.id = i.sid").
		LeftJoin("ssh ON s.id = ssh.server_id").
		Where(squirrel.Eq{
			"team_id":        teamId,
			"allow_terminal": true,
		}).
		OrderBy("weight DESC, id DESC")

	sql, args, err := query.ToSql()
	if err != nil {
		return servers, err
	}

	rows, err := Db.Query(sql, args...)
	if err != nil {
		return servers, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var server _type.Terminal
		if err = rows.Scan(
			&server.ID, &server.Type, &server.Name, &server.Weight, &server.Category,
			&server.OS,
			&server.Username, &server.Address, &server.Port,
		); err != nil {
			return servers, err
		}
		servers = append(servers, server)
	}

	return servers, err
}

func GetTerminalInfo(teamId, serverId int64) (_type.TerminalDetail, error) {
	var server _type.TerminalDetail
	var password []byte
	var keyID int64
	var keyContent []byte
	var keyPassword []byte

	if err := Db.QueryRow(
		`SELECT 
    		s.type, 
    		COALESCE(address, NULL),
			COALESCE(port, NULL),
			COALESCE(username, NULL),
			COALESCE(ssh.password, NULL),
			COALESCE(ssh.key_id, 0),
			COALESCE(keys.content, NULL),
			COALESCE(keys.password, NULL),
			COALESCE(ssh.host_key, NULL),
			COALESCE(ssh.trust_legacy_host_key, false)
		FROM servers s 
		    LEFT JOIN ssh on s.id = ssh.server_id 
			LEFT JOIN keys on ssh.key_id = keys.id
		WHERE s.id = $1 AND s.team_id = $2 AND allow_terminal = true`,
		serverId, teamId,
	).Scan(
		&server.Type,
		&server.Address, &server.Port, &server.Username, &password,
		&keyID, &keyContent, &keyPassword, &server.HostKey, &server.TrustLegacyHostKey,
	); err != nil {
		return server, err
	}

	// Server Password
	if len(password) != 0 {
		pwd, err := encrypt.Decrypt(password, encrypt.Key, encrypt.SSHPasswordContext(serverId))
		if err != nil {
			return server, err
		}
		serverPwd := string(pwd)
		server.Password = &serverPwd
	}

	// Server Key
	if len(keyContent) != 0 {
		key, err := encrypt.Decrypt(keyContent, encrypt.Key, encrypt.KeyContentContext(keyID))
		if err != nil {
			return server, err
		}
		keyStr := string(key)
		server.Key = &keyStr
	}

	// Server Key Password
	if len(keyPassword) != 0 {
		kp, err := encrypt.Decrypt(keyPassword, encrypt.Key, encrypt.KeyPasswordContext(keyID))
		if err != nil {
			return server, err
		}
		keyPwdStr := string(kp)
		server.KeyPwd = &keyPwdStr
	}

	return server, nil
}
