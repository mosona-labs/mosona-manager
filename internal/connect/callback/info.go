package callback

import (
	"fmt"
	"log"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"time"

	"github.com/jmoiron/sqlx"
)

func Information(
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
	tx, err := db.Db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err = updateInformation(tx, serverId, system, startTime, hostName, cpuName, coreC, coreT, kernel, ip, arch, geo); err != nil {
		return err
	}
	return tx.Commit()
}

func AgentInformation(
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
	tx, err := db.Db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err = updateInformation(tx, serverId, system, startTime, hostName, cpuName, coreC, coreT, kernel, ip, arch, geo); err != nil {
		return err
	}
	result, err := tx.Exec(
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
	result, err := tx.Exec(
		"UPDATE server_info SET os = $1, open_time = $2, county = $3, area = $4 WHERE sid = $5",
		system, startTime, geo.CountryCode, geo.Country, serverId,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("server information update affected %d rows", affected)
	}
	result, err = tx.Exec(
		"UPDATE server_info_adv SET hostname = $1, cpu_name = $2, core_c = $3, core_t = $4, kernel = $5, ip = $6, arch = $7 WHERE sid = $8",
		hostName, cpuName, coreC, coreT, kernel, ip, arch, serverId,
	)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return fmt.Errorf("server advanced information update affected %d rows", affected)
	}
	return nil
}
