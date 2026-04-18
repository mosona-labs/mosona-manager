package aserver

import (
	"database/sql"
	"errors"
	"fmt"
	"mosona-manager/internal/_type"
	connectSSH "mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils/encrypt"
)

func validateSSHConnectionForAdd(teamID int64, address string, port int, username, password string, keyID int64) error {
	if address == "" || port == 0 || username == "" {
		return errors.New("incomplete connection information")
	}

	key, keyPwd, err := loadSSHKeyMaterial(teamID, keyID)
	if err != nil {
		return err
	}

	return connectSSH.ValidateConnection(address, port, username, password, key, keyPwd)
}

func validateSSHConnectionForEdit(teamID, serverID int64, data *_type.ServerFullType) error {
	if data.Address == "" || data.Port == 0 || data.Username == "" {
		return errors.New("incomplete connection information")
	}

	var (
		passwordEncrypted []byte
		currentKeyID      int64
	)
	if err := db.Db.QueryRow(
		"SELECT password, key_id FROM ssh WHERE server_id = $1",
		serverID,
	).Scan(&passwordEncrypted, &currentKeyID); err != nil {
		return err
	}

	password := data.Password
	if password == "" && len(passwordEncrypted) != 0 {
		decryptedPassword, err := encrypt.Decrypt(passwordEncrypted, encrypt.Key)
		if err != nil {
			return err
		}
		password = string(decryptedPassword)
	}

	keyID := data.KeyID
	if keyID == 0 {
		keyID = currentKeyID
	}

	key, keyPwd, err := loadSSHKeyMaterial(teamID, keyID)
	if err != nil {
		return err
	}

	return connectSSH.ValidateConnection(data.Address, data.Port, data.Username, password, key, keyPwd)
}

func loadSSHKeyMaterial(teamID, keyID int64) (string, string, error) {
	if keyID == 0 {
		return "", "", nil
	}

	var (
		contentEncrypted  []byte
		passwordEncrypted []byte
	)
	if err := db.Db.QueryRow(
		"SELECT content, password FROM keys WHERE id = $1 AND team_id = $2",
		keyID, teamID,
	).Scan(&contentEncrypted, &passwordEncrypted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", fmt.Errorf("ssh key %d not found", keyID)
		}
		return "", "", err
	}

	key := ""
	if len(contentEncrypted) != 0 {
		decryptedKey, err := encrypt.Decrypt(contentEncrypted, encrypt.Key)
		if err != nil {
			return "", "", err
		}
		key = string(decryptedKey)
	}

	keyPwd := ""
	if len(passwordEncrypted) != 0 {
		decryptedPassword, err := encrypt.Decrypt(passwordEncrypted, encrypt.Key)
		if err != nil {
			return "", "", err
		}
		keyPwd = string(decryptedPassword)
	}

	return key, keyPwd, nil
}
