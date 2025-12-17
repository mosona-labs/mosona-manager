package db

import (
	"mosona-manager/internal/_type"
	"mosona-manager/internal/utils/encrypt"

	"github.com/Masterminds/squirrel"
)

func GetServerInfo(teamId, serverId int64) (_type.ServerFullType, error) {
	var data _type.ServerFullType
	err := Db.Get(
		&data,
		`SELECT 
    		s.id, s.name, s.type, s.allow_monitor, s.allow_terminal,
    		s.weight, s.category,
    		i.note, i.provider, i.cycle, i.start_time, i.end_time, i.amount, i.auto_renew, 
    		i.bandwidth, i.traffic, i.traffic_type, i.note_public
		FROM servers s
		LEFT JOIN server_info i ON s.id = i.sid
		WHERE s.team_id = $1 AND s.id = $2`,
		teamId, serverId,
	)
	if err != nil {
		return data, err
	}

	switch data.Type {
	case 0:
		_ = Db.QueryRow("SELECT address, port, username, key_id FROM ssh WHERE server_id = $1", serverId).Scan(
			&data.Address,
			&data.Port,
			&data.Username,
			&data.KeyID,
		)
	case 1:
		_ = Db.QueryRow("SELECT host, port, agent_uid, status, last_version, last_seen_at FROM agents WHERE server_id = $1", serverId).Scan(
			&data.Address,
			&data.Port,
			&data.AgentUUID,
			&data.AgentStatus,
			&data.AgentVersion,
			&data.AgentLastSeenAt,
		)
	case 2:
		_ = Db.QueryRow("SELECT status, last_version, last_seen_at FROM agents WHERE server_id = $1", serverId).Scan(
			&data.AgentStatus,
			&data.AgentVersion,
			&data.AgentLastSeenAt,
		)
	}

	return data, err
}

func EditServer(teamId, serverId int64, typ int16, data *_type.ServerFullType) error {
	tx, err := Db.Begin()
	if err != nil {
		return err
	}

	// Main
	psql := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	qb := psql.Update("servers").
		Set("name", data.Name).
		Set("allow_monitor", data.AllowMonitor).
		Set("allow_terminal", data.AllowTerminal).
		Set("weight", data.Weight).
		Set("category", data.Category).
		Where(squirrel.Eq{"id": serverId, "team_id": teamId})
	query, args, err := qb.ToSql()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(query, args...)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	// Info
	psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	qb = psql.Update("server_info").
		Set("note", data.Note).
		Set("provider", data.Provider).
		Set("cycle", data.Cycle).
		Set("start_time", data.StartTime).
		Set("end_time", data.EndTime).
		Set("amount", data.Amount).
		Set("auto_renew", data.AutoRenew).
		Set("bandwidth", data.Bandwidth).
		Set("traffic", data.Traffic).
		Set("traffic_type", data.TrafficType).
		Set("note_public", data.NotePublic).
		Where(squirrel.Eq{"sid": serverId})
	query, args, err = qb.ToSql()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(query, args...)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	// Connection
	switch typ {
	case 0:
		psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
		qb = psql.Update("ssh").
			Set("address", data.Address).
			Set("port", data.Port).
			Set("username", data.Username).
			Where(squirrel.Eq{"id": serverId})

		if data.KeyID != 0 {
			qb = qb.Set("key_id", data.KeyID)
		}
		if data.Password != "" {
			pwd, err := encrypt.Encrypt([]byte(data.Password), encrypt.Key)
			if err != nil {
				_ = tx.Rollback()
				return err
			}
			qb = qb.Set("password", pwd)
		}
	}
	query, args, err = qb.ToSql()
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_, err = tx.Exec(query, args...)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	// Commit
	if err = tx.Commit(); err != nil {
		return err
	}

	return nil
}

func DeleteServer(teamId, serverId int64) error {
	_, err := Db.Exec("DELETE FROM servers WHERE id = $1 AND team_id = $2", serverId, teamId)
	return err
}
