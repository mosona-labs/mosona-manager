package db

import (
	"errors"
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

type agentPrivateKeyRow struct {
	ServerID   int64  `db:"server_id"`
	PrivateKey []byte `db:"private_key"`
}

type CredentialMigrationFailure struct {
	Table    string
	RecordID int64
	Field    string
	Err      error
}

type CredentialMigrationReport struct {
	Failures []CredentialMigrationFailure
}

type ciphertextMigrationError struct {
	operation string
	err       error
}

func (e *ciphertextMigrationError) Error() string { return e.operation + ": " + e.err.Error() }
func (e *ciphertextMigrationError) Unwrap() error { return e.err }

// MigrateEncryptedCredentials atomically upgrades legacy AES-CBC credentials
// and plaintext Agent keys after the master key has been loaded and before
// application workers start.
func MigrateEncryptedCredentials() (CredentialMigrationReport, error) {
	var report CredentialMigrationReport
	tx, err := Db.Beginx()
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback() }()

	var keys []encryptedKeyRow
	if err = tx.Select(&keys, "SELECT id, content, password FROM keys ORDER BY id FOR UPDATE"); err != nil {
		return report, fmt.Errorf("load encrypted keys for migration: %w", err)
	}
	for _, row := range keys {
		content, changed, err := migrateCiphertext(row.Content, encrypt.KeyContentContext(row.ID))
		if err != nil {
			if !isCiphertextDecryptionError(err) {
				return report, fmt.Errorf("migrate key %d content: %w", row.ID, err)
			}
			report.Failures = append(report.Failures, CredentialMigrationFailure{
				Table: "keys", RecordID: row.ID, Field: "content", Err: err,
			})
			content, changed = row.Content, false
		}
		password, passwordChanged, err := migrateCiphertext(row.Password, encrypt.KeyPasswordContext(row.ID))
		if err != nil {
			if !isCiphertextDecryptionError(err) {
				return report, fmt.Errorf("migrate key %d password: %w", row.ID, err)
			}
			report.Failures = append(report.Failures, CredentialMigrationFailure{
				Table: "keys", RecordID: row.ID, Field: "password", Err: err,
			})
			password, passwordChanged = row.Password, false
		}
		if !changed && !passwordChanged {
			continue
		}
		if _, err = tx.Exec(
			"UPDATE keys SET content = $1, password = $2 WHERE id = $3",
			content, nullableCiphertext(password), row.ID,
		); err != nil {
			return report, fmt.Errorf("update migrated key %d: %w", row.ID, err)
		}
	}

	var sshRows []encryptedSSHRow
	if err = tx.Select(&sshRows, "SELECT server_id, password FROM ssh ORDER BY server_id FOR UPDATE"); err != nil {
		return report, fmt.Errorf("load encrypted SSH passwords for migration: %w", err)
	}
	for _, row := range sshRows {
		password, changed, err := migrateCiphertext(row.Password, encrypt.SSHPasswordContext(row.ServerID))
		if err != nil {
			if !isCiphertextDecryptionError(err) {
				return report, fmt.Errorf("migrate SSH password for server %d: %w", row.ServerID, err)
			}
			report.Failures = append(report.Failures, CredentialMigrationFailure{
				Table: "ssh", RecordID: row.ServerID, Field: "password", Err: err,
			})
			continue
		}
		if !changed {
			continue
		}
		if _, err = tx.Exec("UPDATE ssh SET password = $1 WHERE server_id = $2", password, row.ServerID); err != nil {
			return report, fmt.Errorf("update migrated SSH password for server %d: %w", row.ServerID, err)
		}
	}

	var agentRows []agentPrivateKeyRow
	if err = tx.Select(&agentRows, "SELECT server_id, private_key FROM agents ORDER BY server_id FOR UPDATE"); err != nil {
		return report, fmt.Errorf("load Agent private keys for migration: %w", err)
	}
	for _, row := range agentRows {
		privateKey, changed, err := migrateAgentPrivateKey(row.PrivateKey, row.ServerID)
		if err != nil {
			if !isCiphertextDecryptionError(err) {
				return report, fmt.Errorf("migrate Agent private key for server %d: %w", row.ServerID, err)
			}
			report.Failures = append(report.Failures, CredentialMigrationFailure{
				Table: "agents", RecordID: row.ServerID, Field: "private_key", Err: err,
			})
			continue
		}
		if !changed {
			continue
		}
		if _, err = tx.Exec("UPDATE agents SET private_key = $1 WHERE server_id = $2", privateKey, row.ServerID); err != nil {
			return report, fmt.Errorf("update migrated Agent private key for server %d: %w", row.ServerID, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return report, fmt.Errorf("commit credential encryption migration: %w", err)
	}
	return report, nil
}

func migrateAgentPrivateKey(privateKey []byte, serverID int64) ([]byte, bool, error) {
	if len(privateKey) == 0 {
		return privateKey, false, nil
	}
	context := encrypt.AgentPrivateKeyContext(serverID)
	if encrypt.IsVersionedCiphertext(privateKey) {
		if _, err := encrypt.Decrypt(privateKey, encrypt.Key, context); err != nil {
			return nil, false, &ciphertextMigrationError{operation: "decrypt current Agent private key", err: err}
		}
		return privateKey, false, nil
	}
	migrated, err := encrypt.Encrypt(privateKey, encrypt.Key, context)
	if err != nil {
		return nil, false, err
	}
	return migrated, true, nil
}

func migrateCiphertext(ciphertext []byte, context string) ([]byte, bool, error) {
	if len(ciphertext) == 0 {
		return ciphertext, false, nil
	}
	if encrypt.IsVersionedCiphertext(ciphertext) {
		if _, err := encrypt.Decrypt(ciphertext, encrypt.Key, context); err != nil {
			return nil, false, &ciphertextMigrationError{operation: "decrypt current ciphertext", err: err}
		}
		return ciphertext, false, nil
	}
	plaintext, err := encrypt.DecryptLegacy(ciphertext, encrypt.Key)
	if err != nil {
		return nil, false, &ciphertextMigrationError{operation: "decrypt legacy ciphertext", err: err}
	}
	migrated, err := encrypt.Encrypt(plaintext, encrypt.Key, context)
	if err != nil {
		return nil, false, err
	}
	return migrated, true, nil
}

func isCiphertextDecryptionError(err error) bool {
	var migrationErr *ciphertextMigrationError
	return errors.As(err, &migrationErr) && migrationErr.operation != ""
}

func nullableCiphertext(ciphertext []byte) any {
	if len(ciphertext) == 0 {
		return nil
	}
	return ciphertext
}
