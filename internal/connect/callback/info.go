package callback

import (
	"log"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"time"
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
) {
	if _, err := db.Db.Exec(
		"UPDATE server_info SET os = $1, open_time = $2 WHERE sid = $3",
		system, startTime, serverId,
	); err != nil {
		log.Println("Failed to update server info:", err)
	}
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

	tx, err := db.Db.Begin()
	if err != nil {
		log.Println("Failed to begin transaction:", err)
		return
	}
	if _, err = tx.Exec(
		"UPDATE server_info SET county = $1, area = $2 WHERE sid = $3",
		geo.CountryCode, geo.Country, serverId,
	); err != nil {
		_ = tx.Rollback()
		log.Println("Failed to update server info:", err)
		return
	}
	if _, err = tx.Exec(
		"UPDATE server_info_adv SET hostname = $1, cpu_name = $2, core_c = $3, core_t = $4, kernel = $5, ip = $6, arch = $7 WHERE sid = $8",
		hostName, cpuName, coreC, coreT, kernel, ip, arch, serverId,
	); err != nil {
		_ = tx.Rollback()
		log.Println("Failed to update server advanced info:", err)
		return
	}
	if err = tx.Commit(); err != nil {
		log.Println("Failed to commit transaction:", err)
		return
	}
}
