package connect

import (
	"context"
	"log"
	"mosona-manager/_type"
	"mosona-manager/config"
	"mosona-manager/db"
	"mosona-manager/influx"
	"mosona-manager/utils"
	"sync"
	"time"
)

var (
	mu          sync.Mutex
	connectPool = make(map[int64]*serverEntry)
)

type infoCallbackType func(
	system string,
	bootTime time.Time,
	hostName string,
	cpuName string,
	coreC int,
	coreT int,
	kernel string,
	ip string,
	arch string,
)

type serverEntry struct {
	cancel   context.CancelFunc
	host     string
	port     int
	user     string
	password string
	key      string
	keyPwd   string
	callback func(data _type.ServerStatusType)
	info     infoCallbackType
}

func StartServer(serverId int64) error {
	var host, user string
	var port int

	var (
		password []byte
		pwdStr   string
	)
	var (
		key    []byte
		keyStr string
	)
	var (
		keyPassword []byte
		keyPwdStr   string
	)

	if err := db.Db.QueryRow(
		"SELECT address, port, username, s.password, k.password, k.content FROM servers s LEFT JOIN keys k ON s.key_id = k.id WHERE s.id = $1", serverId,
	).Scan(&host, &port, &user, &password, &keyPassword, &key); err != nil {
		return err
	}
	if password != nil {
		pwd, err := utils.Decrypt(password, config.Key)
		if err != nil {
			return err
		}
		pwdStr = string(pwd)
	}
	if key != nil {
		k, err := utils.Decrypt(key, config.Key)
		if err != nil {
			return err
		}
		keyStr = string(k)
	}
	if keyPassword != nil {
		kp, err := utils.Decrypt(keyPassword, config.Key)
		if err != nil {
			return err
		}
		keyPwdStr = string(kp)
	}

	var callback = func(data _type.ServerStatusType) {
		if err := influx.AddServerStatus(serverId, data); err != nil {
			log.Println("Failed to add server status:", err)
		}
	}
	var info = func(
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
		if ip == "Unknown" {
			var err error
			ip, err = utils.GetDomainAddress(host)
			if err != nil {
				log.Println("Failed to get domain address:", err)
			}
		}
		geo, err := utils.GetIPGeoLocation(ip)
		if err != nil {
			log.Println("Failed to get IP geo location:", err)
			geo.CountryCode = "UN"
			geo.Country = "Unknown"
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

	mu.Lock()
	if old, exists := connectPool[serverId]; exists {
		old.cancel()
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &serverEntry{
			cancel:   cancel,
			host:     host,
			port:     port,
			user:     user,
			password: pwdStr,
			key:      keyStr,
			keyPwd:   keyPwdStr,
			callback: callback,
			info:     info,
		}
		mu.Unlock()
		go func() {
			_ = SSH(ctx, host, port, user, pwdStr, keyStr, keyPwdStr, callback, info)
		}()
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		connectPool[serverId] = &serverEntry{
			cancel:   cancel,
			host:     host,
			port:     port,
			user:     user,
			password: pwdStr,
			key:      keyStr,
			keyPwd:   keyPwdStr,
			callback: callback,
			info:     info,
		}
		mu.Unlock()
		go func() {
			_ = SSH(ctx, host, port, user, pwdStr, keyStr, keyPwdStr, callback, info)
		}()
	}

	return nil
}

func StopServer(serverId int64) {
	mu.Lock()
	if entry, exists := connectPool[serverId]; exists {
		entry.cancel()
		delete(connectPool, serverId)
	}
	mu.Unlock()
}
