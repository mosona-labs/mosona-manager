package db

import (
	"fmt"
	"mosona-manager/internal/utils/encrypt"
)

type encryptedKeyRow struct {
	ID       int64  `db:"id"`
	Content  []byte `db:"content"`
	Password []byte `db:"password"`
}

type encryptedSSHRow struct {
	ServerID int64  `db:"server_id"`
	Password []byte `db:"password"`
}

// MigrateEncryptedCredentials atomically upgrades legacy AES-CBC credentials
// after the master key has been loaded and before application workers start.
func MigrateEncryptedCredentials() error {
	tx, err := Db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var keys []encryptedKeyRow
	if err = tx.Select(&keys, "SELECT id, content, password FROM keys ORDER BY id FOR UPDATE"); err != nil {
		return fmt.Errorf("load encrypted keys for migration: %w", err)
	}
	for _, row := range keys {
		content, changed, err := migrateCiphertext(row.Content, encrypt.KeyContentContext(row.ID))
		if err != nil {
			return fmt.Errorf("migrate key %d content: %w", row.ID, err)
		}
		password, passwordChanged, err := migrateCiphertext(row.Password, encrypt.KeyPasswordContext(row.ID))
		if err != nil {
			return fmt.Errorf("migrate key %d password: %w", row.ID, err)
		}
		if !changed && !passwordChanged {
			continue
		}
		if _, err = tx.Exec(
			"UPDATE keys SET content = $1, password = $2 WHERE id = $3",
			content, nullableCiphertext(password), row.ID,
		); err != nil {
			return fmt.Errorf("update migrated key %d: %w", row.ID, err)
		}
	}

	var sshRows []encryptedSSHRow
	if err = tx.Select(&sshRows, "SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE"); err != nil {
		return fmt.Errorf("load encrypted SSH passwords for migration: %w", err)
	}
	for _, row := range sshRows {
		password, changed, err := migrateCiphertext(row.Password, encrypt.SSHPasswordContext(row.ServerID))
		if err != nil {
			return fmt.Errorf("migrate SSH password for server %d: %w", row.ServerID, err)
		}
		if !changed {
			continue
		}
		if _, err = tx.Exec("UPDATE ssh SET password = $1 WHERE server_id = $2", password, row.ServerID); err != nil {
			return fmt.Errorf("update migrated SSH password for server %d: %w", row.ServerID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit credential encryption migration: %w", err)
	}
	return nil
}

func migrateCiphertext(ciphertext []byte, context string) ([]byte, bool, error) {
	if len(ciphertext) == 0 {
		return ciphertext, false, nil
	}
	if encrypt.IsVersionedCiphertext(ciphertext) {
		if _, err := encrypt.Decrypt(ciphertext, encrypt.Key, context); err != nil {
			return nil, false, err
		}
		return ciphertext, false, nil
	}
	plaintext, err := encrypt.DecryptLegacy(ciphertext, encrypt.Key)
	if err != nil {
		return nil, false, err
	}
	migrated, err := encrypt.Encrypt(plaintext, encrypt.Key, context)
	if err != nil {
		return nil, false, err
	}
	return migrated, true, nil
}

func nullableCiphertext(ciphertext []byte) any {
	if len(ciphertext) == 0 {
		return nil
	}
	return ciphertext
}
