package db

import (
	"fmt"

	"mosona-manager/internal/utils/encrypt"
)

func HasEncryptedCredentials() (bool, error) {
	var exists bool
	magic := encrypt.EnvelopeMagic()
	err := Db.Get(&exists, fmt.Sprintf(`
SELECT EXISTS (SELECT 1 FROM keys)
    OR EXISTS (SELECT 1 FROM ssh)
    OR EXISTS (
        SELECT 1
        FROM agents
        WHERE substring(private_key FROM 1 FOR %d) = convert_to($1, 'UTF8')
    )`, len(magic)), magic)
	return exists, err
}
