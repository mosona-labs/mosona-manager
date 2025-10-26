package db

import (
	"mosona-manager/_type"
)

func GetAllTeamPlans() ([]_type.TeamPlan, error) {
	var plans = make([]_type.TeamPlan, 0)
	if err := Db.Select(&plans, "SELECT id, name, description, price, max_server, max_alert, max_member, created_at, updated_at FROM team_plans ORDER BY price"); err != nil {
		return nil, err
	}

	return plans, nil
}

func GetPlanById(id int64) (_type.TeamPlan, error) {
	var plan _type.TeamPlan
	if err := Db.Get(&plan, "SELECT id, name, description, price, max_server, max_alert, max_member, created_at, updated_at FROM team_plans WHERE id = $1", id); err != nil {
		return _type.TeamPlan{}, err
	}

	return plan, nil
}
