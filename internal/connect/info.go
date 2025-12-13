package connect

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/script"
	"mosona-manager/internal/db"
	"mosona-manager/internal/utils"
	"time"

	"golang.org/x/crypto/ssh"
)

func information(client *ssh.Client, callback ServerInfoCallback) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	reader := io.MultiReader(stdout, stderr)
	scanner := bufio.NewScanner(reader)
	done := make(chan struct{})
	go func() {
		for scanner.Scan() {
			var data _type.ServerInfoType
			if err = json.Unmarshal([]byte(scanner.Text()), &data); err != nil {
				continue
			}
			callback(data)
		}
		close(done)
	}()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	if err = session.Start("bash -s"); err != nil {
		return err
	}

	scriptFile, err := script.GetScript("linux_info.sh")
	if err != nil {
		_ = stdin.Close()
		return err
	}
	_, _ = io.WriteString(stdin, scriptFile)
	_ = stdin.Close()

	if err = session.Wait(); err != nil {
		<-done
		return err
	}
	<-done
	return nil
}

func CallbackInformation(
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
