package db

import (
	"context"
	"database/sql"
	"errors"
	"mosona-manager/internal/_type"
)

var ErrSameCategoryName = errors.New("duplicate category name")
var ErrCanNotDeleteDefaultCategory = errors.New("cannot delete default category")

func GetCategoryById(teamId, categoryId int64) (_type.Category, error) {
	var category _type.Category
	err := Db.Get(
		&category,
		"SELECT id, name, sort FROM categories WHERE team = $1 AND id = $2",
		teamId, categoryId,
	)
	return category, err
}

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
	defer func() { _ = tx.Rollback() }()

	var defaultCategoryId int64
	if err = tx.QueryRow(
		"SELECT id FROM categories WHERE team = $1 ORDER BY id LIMIT 1",
		teamId,
	).Scan(&defaultCategoryId); err != nil {
		return err
	}
	if defaultCategoryId == categoryId {
		return ErrCanNotDeleteDefaultCategory
	}

	// Reassign servers to default category
	if _, err = tx.Exec(
		"UPDATE servers SET category = $1 WHERE category = $2 AND team_id = $3",
		defaultCategoryId, categoryId, teamId,
	); err != nil {
		return err
	}

	// Delete category
	result, err := tx.Exec("DELETE FROM categories WHERE id = $1 AND team = $2", categoryId, teamId)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return sql.ErrNoRows
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

func SortCategories(teamId int64, categoryIds []int64) error {
	tx, err := Db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for index, categoryId := range categoryIds {
		if _, err = tx.Exec(
			"UPDATE categories SET sort = $1 WHERE id = $2 AND team = $3",
			index, categoryId, teamId,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func GetCategoriesByTeam(teamId int64) ([]_type.Category, error) {
	return GetCategoriesByTeamContext(context.Background(), teamId)
}

func GetCategoriesByTeamContext(ctx context.Context, teamId int64) ([]_type.Category, error) {
	var categories = make([]_type.Category, 0)
	err := Db.SelectContext(ctx, &categories, "SELECT id, name, sort FROM categories WHERE team = $1 ORDER BY sort, id", teamId)
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
