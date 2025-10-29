package db

import (
	"errors"
	"mosona-manager/_type"
)

var ErrSameCategoryName = errors.New("duplicate category name")
var ErrCanNotDeleteDefaultCategory = errors.New("cannot delete default category")

func CreateCategory(teamId int64, name string) error {
	var exist int
	err := Db.Get(&exist, "SELECT COUNT(*) FROM categories WHERE team = $1 AND name = $2", teamId, name)
	if err != nil {
		return err
	}
	if exist > 0 {
		return ErrSameCategoryName
	}

	_, err = Db.Exec("INSERT INTO categories (team, name) VALUES ($1, $2)", teamId, name)
	return err
}

func DeleteCategory(teamId, categoryId int64) error {
	tx, err := Db.Begin()
	if err != nil {
		return err
	}

	var defaultCategoryId int64
	if err = Db.Get(
		&defaultCategoryId,
		"SELECT id FROM categories WHERE team = $1 ORDER BY id LIMIT 1",
		teamId,
	); err != nil {
		return err
	}
	if defaultCategoryId == categoryId {
		return ErrCanNotDeleteDefaultCategory
	}

	// Reassign servers to default category
	if _, err = tx.Exec(
		"UPDATE servers SET category = $1 WHERE category = $2",
		defaultCategoryId, categoryId, teamId,
	); err != nil {
		_ = tx.Rollback()
		return err
	}

	// Delete category
	if _, err = Db.Exec("DELETE FROM categories WHERE id = $1 AND team = $2", categoryId, teamId); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func EditCategory(teamId, categoryId int64, name string) error {
	var exist int
	err := Db.Get(&exist, "SELECT COUNT(*) FROM categories WHERE team = $1 AND name = $2 AND id != $3", teamId, name, categoryId)
	if err != nil {
		return err
	}
	if exist > 0 {
		return ErrSameCategoryName
	}

	_, err = Db.Exec("UPDATE categories SET name = $1 WHERE id = $2 AND team = $3", name, categoryId, teamId)
	return err
}

func GetCategoriesByTeam(teamId int64) ([]_type.Category, error) {
	var categories = make([]_type.Category, 0)
	err := Db.Select(&categories, "SELECT id, name FROM categories WHERE team = $1 ORDER BY id", teamId)
	if err != nil {
		return nil, err
	}

	return categories, nil
}

func SetServerCategory(teamId, serverId, categoryId int64) error {
	_, err := Db.Exec(
		`UPDATE servers SET category = $1 WHERE id = $2 AND team_id = $3`,
		categoryId, serverId, teamId,
	)
	return err
}
