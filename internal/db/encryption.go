package db

func HasEncryptedCredentials() (bool, error) {
	var exists bool
	err := Db.Get(&exists, `
SELECT EXISTS (SELECT 1 FROM keys)
    OR EXISTS (SELECT 1 FROM ssh)`)
	return exists, err
}
