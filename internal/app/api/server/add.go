package aserver

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/connect/conn"
	connectSSH "mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"mosona-manager/internal/utils/encrypt"
	"mosona-manager/pkg/identity"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

var (
	addNewAgentUUID       = uuid.NewRandom
	addRemoveServerStatus = influx.RemoveServerStatus
	addReconcileServer    = conn.ReconcileServer
	addLogAdd             = influx.LogAdd
)

func add(c *echo.Context) error {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)

	name := c.FormValue("name")
	mode, _ := strconv.Atoi(c.FormValue("mode"))
	categoryId, _ := strconv.ParseInt(c.FormValue("category_id"), 10, 64)
	allowMonitor := c.FormValue("allow_monitor") == "true"
	allowTerminal := c.FormValue("allow_terminal") == "true"
	publicVisible := c.FormValue("public_visible") != "false"

	if tid == 0 || name == "" || categoryId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}
	if _, err := db.GetCategoryById(tid, categoryId); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Invalid category",
			})
		}
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Display
	weight, _ := strconv.Atoi(c.FormValue("weight"))
	note := c.FormValue("note")

	// Bill
	provider := c.FormValue("provider")
	cycle, _ := strconv.Atoi(c.FormValue("cycle"))
	startTime := c.FormValue("start_time")
	endTime := c.FormValue("end_time")
	amount := c.FormValue("amount")
	autoRenew := c.FormValue("auto_renew") == "true"

	var startTimeParsed sql.NullTime
	if startTime != "" {
		t, err := time.Parse(time.RFC3339, startTime)
		if err != nil {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Invalid start_time format",
			})
		}
		startTimeParsed = sql.NullTime{Time: t, Valid: true}
	} else {
		startTimeParsed = sql.NullTime{Valid: false}
	}
	var endTimeParsed sql.NullTime
	if endTime != "" {
		t, err := time.Parse(time.RFC3339, endTime)
		if err != nil {
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Invalid end_time format",
			})
		}
		endTimeParsed = sql.NullTime{Time: t, Valid: true}
	} else {
		endTimeParsed = sql.NullTime{Valid: false}
	}

	// Network
	bandwidth := c.FormValue("bandwidth")
	traffic := c.FormValue("traffic")
	trafficType, _ := strconv.Atoi(c.FormValue("traffic_type"))
	notePublic := c.FormValue("note_public")

	var validatedHostKey string
	if mode == 0 {
		address := c.FormValue("address")
		port, _ := strconv.Atoi(c.FormValue("port"))
		username := c.FormValue("username")
		password := c.FormValue("password")
		keyID, _ := strconv.ParseInt(c.FormValue("key_id"), 10, 64)

		validation, err := validateSSHConnectionForAdd(tid, address, port, username, password, keyID, c.FormValue("host_key"))
		if err != nil {
			if errors.Is(err, connectSSH.ErrHostKeyChangedDuringValidation) {
				addLogAdd(
					tid, uid, "security", "Blocked SSH host key change while validating new Server: "+name,
					c.RealIP(), c.Request().UserAgent(), "high",
				)
			}
			if connectSSH.IsHostKeyTrustError(err) {
				return sshHostKeyConfirmationRequired(c, validation)
			}
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "SSH connection failed: " + err.Error(),
			})
		}
		validatedHostKey = validation.HostKey
	}

	// Insert into database
	tx, err := db.Db.Begin()
	if err != nil {
		return c.JSON(500, _type.H{
			Code: "error",
			Msg:  "Database error",
		})
	}
	defer func() { _ = tx.Rollback() }()
	var serverId int64
	if err = tx.QueryRow(`INSERT INTO servers (team_id, name, type, category, allow_monitor, allow_terminal, public_visible, weight) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		tid, name, mode, categoryId, allowMonitor, allowTerminal, publicVisible, weight,
	).Scan(&serverId); err != nil {
		_ = tx.Rollback()
		return utils.ErrorHandler(c, err, "Database error")
	}
	if _, err = tx.Exec(
		"INSERT INTO server_info (sid, note, provider, cycle, start_time, end_time, amount, auto_renew, bandwidth, traffic, traffic_type, note_public) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)",
		serverId, note, provider, cycle, startTimeParsed, endTimeParsed, amount, autoRenew, bandwidth, traffic, trafficType, notePublic,
	); err != nil {
		_ = tx.Rollback()
		return utils.ErrorHandler(c, err, "Database error")
	}
	if _, err = tx.Exec(
		"INSERT INTO server_info_adv (sid) VALUES ($1)",
		serverId,
	); err != nil {
		_ = tx.Rollback()
		return utils.ErrorHandler(c, err, "Database error")
	}

	// Response
	var response _type.Map

	// Connection
	switch mode {
	case 0: // SSH
		address := c.FormValue("address")
		port, _ := strconv.Atoi(c.FormValue("port"))
		username := c.FormValue("username")
		password := c.FormValue("password")
		keyID, _ := strconv.ParseInt(c.FormValue("key_id"), 10, 64)

		if address == "" || port == 0 || username == "" {
			_ = tx.Rollback()
			return c.JSON(400, _type.H{
				Code: "error",
				Msg:  "Incomplete connection information",
			})
		}

		// Encrypt password
		passwordEncrypt, err := encrypt.Encrypt([]byte(password), encrypt.Key, encrypt.SSHPasswordContext(serverId))
		if err != nil {
			return utils.ErrorHandler(c, err, "Encryption error")
		}
		if _, err = tx.Exec(
			"INSERT INTO ssh (server_id, address, port, username, key_id, password, host_key) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			serverId, address, port, username, keyID, passwordEncrypt, validatedHostKey,
		); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}
		response = _type.Map{
			"id": serverId,
		}
	case 1: // Agent (active)
		agentUUID, err := addNewAgentUUID()
		if err != nil {
			return utils.ErrorHandler(c, err, "Agent ID generation error")
		}
		privateKey, publicKey, err := identity.GenerateEd25519KeyPair()
		if err != nil {
			return utils.ErrorHandler(c, err, "Key generation error")
		}
		privateKeyEncrypted, err := encrypt.Encrypt([]byte(privateKey), encrypt.Key, encrypt.AgentPrivateKeyContext(serverId))
		if err != nil {
			return utils.ErrorHandler(c, err, "Encryption error")
		}

		address := c.FormValue("address")
		port, _ := strconv.Atoi(c.FormValue("port"))
		if _, err = tx.Exec(
			"INSERT INTO agents (server_id, agent_uid, status, host, port, private_key) VALUES ($1, $2, $3, $4, $5, $6)",
			serverId, agentUUID.String(), 0, address, port, privateKeyEncrypted,
		); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}

		response = _type.Map{
			"id":         serverId,
			"host":       address,
			"port":       port,
			"agent_uid":  agentUUID.String(),
			"public_key": base64.StdEncoding.EncodeToString([]byte(publicKey)),
		}
	case 2: // Agent (passive)
		dynamicConf := config.ReadDynamicConf()
		enrollToken := utils.RandomString(32)
		tokenHash := utils.SHA256(enrollToken + dynamicConf.Token)
		if _, err = tx.Exec(
			"INSERT INTO enroll_tokens (server_id, token_hash) VALUES ($1, $2)",
			serverId, tokenHash,
		); err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}

		response = _type.Map{
			"id":           serverId,
			"hub":          dynamicConf.Domain,
			"enroll_token": enrollToken,
		}
	}

	// Team alerts
	teamAlerts, err := db.GetTeamAlertsByTeamIdTx(tx, tid)
	if err != nil {
		_ = tx.Rollback()
		return utils.ErrorHandler(c, err, "Database error")
	}
	for _, alert := range teamAlerts {
		_, err = tx.Exec(`
			INSERT INTO server_alerts (server_id, item, threshold, for_duration)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (server_id, item) DO UPDATE
			SET threshold = EXCLUDED.threshold,
			    for_duration = EXCLUDED.for_duration
		`, serverId, alert.Item, alert.Threshold, alert.ForDuration)
		if err != nil {
			_ = tx.Rollback()
			return utils.ErrorHandler(c, err, "Database error")
		}
	}

	if err = tx.Commit(); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	if err = addRemoveServerStatus(serverId); err != nil {
		if reconcileErr := addReconcileServer(serverId); reconcileErr != nil {
			log.Println("Failed to reconcile new server connection:", reconcileErr)
		}
		return utils.ErrorHandler(c, err, "Failed to clear server status from InfluxDB")
	}
	if reconcileErr := addReconcileServer(serverId); reconcileErr != nil {
		log.Println("Failed to reconcile new server connection:", reconcileErr)
	}

	// Log action
	addLogAdd(
		tid, uid, "server", "Create Server: "+name+" (ID"+strconv.FormatInt(serverId, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)

	return c.JSON(200, _type.H{
		Msg:  "Server added",
		Data: response,
	})
}
