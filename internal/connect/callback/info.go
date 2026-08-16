package callback

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"time"

	"github.com/jmoiron/sqlx"
)

const informationTransactionTimeout = 15 * time.Second

func Information(
	ctx context.Context,
	serverId int64,
	host string,
	system string,
	startTime time.Time,
	hostName string,
	cpuName string,
	coreC int,
	coreT int,
	kernel string,
	ip string,
	arch string,
) error {
	geo, ip := resolveLocation(host, ip)
	ctx, cancel := context.WithTimeout(ctx, informationTransactionTimeout)
	defer cancel()
	tx, err := db.Db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err = updateInformation(ctx, tx, serverId, system, startTime, hostName, cpuName, coreC, coreT, kernel, ip, arch, geo); err != nil {
		return err
	}
	return tx.Commit()
}

func AgentInformation(
	ctx context.Context,
	serverId int64,
	host string,
	system string,
	startTime time.Time,
	hostName string,
	cpuName string,
	coreC int,
	coreT int,
	kernel string,
	ip string,
	arch string,
	version string,
) error {
	geo, ip := resolveLocation(host, ip)
	ctx, cancel := context.WithTimeout(ctx, informationTransactionTimeout)
	defer cancel()
	tx, err := db.Db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err = updateInformation(ctx, tx, serverId, system, startTime, hostName, cpuName, coreC, coreT, kernel, ip, arch, geo); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx,
		"UPDATE agents SET status = 1, last_ip = $1, last_version = $2, last_seen_at = NOW() WHERE server_id = $3",
		ip, version, serverId,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("agent information update affected %d rows", affected)
	}
	return tx.Commit()
}

func resolveLocation(host, ip string) (utils.IPGeoResponse, string) {
	if ip == "Unknown" && host != "" {
		var err error
		ip, err = utils.GetDomainAddress(host)
		if err != nil {
			log.Println("Failed to get domain address:", err)
		}
	}
	geo, err := utils.GetIPGeoLocation(ip)
	if err != nil {
		if ip != "" {
			log.Println("Failed to get IP geo location:", err, ip)
		} else {
			log.Println("IP is empty, cannot get geo location")
		}
		geo = utils.IPGeoResponse{
			CountryCode: "UN",
			Country:     "Unknown",
		}
	}
	return geo, ip
}

func updateInformation(
	ctx context.Context,
	tx *sqlx.Tx,
	serverId int64,
	system string,
	startTime time.Time,
	hostName string,
	cpuName string,
	coreC int,
	coreT int,
	kernel string,
	ip string,
	arch string,
	geo utils.IPGeoResponse,
) error {
	result, err := tx.ExecContext(ctx,
		`UPDATE server_info
		 SET os = $1, open_time = $2, county = $3, area = $4
		 WHERE sid = $5
		   AND (os IS DISTINCT FROM $1
		     OR open_time IS DISTINCT FROM $2
		     OR county IS DISTINCT FROM $3
		     OR area IS DISTINCT FROM $4)`,
		system, startTime, geo.CountryCode, geo.Country, serverId,
	)
	if err != nil {
		return err
	}
	if err = ensureConditionalUpdateTarget(ctx, tx, result, "server information", "SELECT EXISTS (SELECT 1 FROM server_info WHERE sid = $1)", serverId); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx,
		`UPDATE server_info_adv
		 SET hostname = $1, cpu_name = $2, core_c = $3, core_t = $4, kernel = $5, ip = $6, arch = $7
		 WHERE sid = $8
		   AND (hostname IS DISTINCT FROM $1
		     OR cpu_name IS DISTINCT FROM $2
		     OR core_c IS DISTINCT FROM $3
		     OR core_t IS DISTINCT FROM $4
		     OR kernel IS DISTINCT FROM $5
		     OR ip IS DISTINCT FROM $6
		     OR arch IS DISTINCT FROM $7)`,
		hostName, cpuName, coreC, coreT, kernel, ip, arch, serverId,
	)
	if err != nil {
		return err
	}
	return ensureConditionalUpdateTarget(ctx, tx, result, "server advanced information", "SELECT EXISTS (SELECT 1 FROM server_info_adv WHERE sid = $1)", serverId)
}

func ensureConditionalUpdateTarget(
	ctx context.Context,
	tx *sqlx.Tx,
	result sql.Result,
	label string,
	existsQuery string,
	serverID int64,
) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 1 {
		return nil
	}
	if affected != 0 {
		return fmt.Errorf("%s update affected %d rows", label, affected)
	}

	var exists bool
	if err = tx.GetContext(ctx, &exists, existsQuery, serverID); err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s update affected 0 rows", label)
	}
	return nil
}
