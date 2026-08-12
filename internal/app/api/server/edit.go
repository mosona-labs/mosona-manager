package aserver

import (
	"errors"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/connect/conn"
	connectSSH "mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/utils"
	"strconv"

	"github.com/labstack/echo/v5"
)

func edit(c *echo.Context) error {
	uid, _ := c.Get("uid").(int64)
	tid, _ := c.Get("tid").(int64)
	serverId, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if tid == 0 || serverId == 0 {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid server data",
		})
	}

	var data _type.ServerFullType
	if err := c.Bind(&data); err != nil {
		return c.JSON(400, _type.H{
			Code: "error",
			Msg:  "Invalid request data",
		})
	}

	var typ int16
	var lastAllowMonitor bool
	if err := db.Db.QueryRow(
		"SELECT type, allow_monitor FROM servers WHERE id=$1 AND team_id=$2",
		serverId, tid,
	).Scan(&typ, &lastAllowMonitor); err != nil {
		return utils.ErrorHandler(c, err, "Database error")
	}

	hostKeyChanged := false
	if typ == 0 {
		validation, err := validateSSHConnectionForEdit(tid, serverId, &data)
		if err != nil {
			if validation.Changed || errors.Is(err, connectSSH.ErrHostKeyChangedDuringValidation) {
				influx.LogAdd(
					tid, uid, "security", "Blocked changed SSH host key for Server: "+data.Name+" (ID"+strconv.FormatInt(serverId, 10)+")",
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
		data.HostKey = validation.HostKey
		data.PreviousHostKey = validation.PreviousHostKey
		hostKeyChanged = validation.Changed
	}

	if err := db.EditServer(tid, serverId, typ, &data); err != nil {
		if errors.Is(err, db.ErrSSHHostKeyStateChanged) {
			return c.JSON(409, _type.H{
				Code: "ssh_host_key_state_changed",
				Msg:  "SSH host key trust changed while the server was being edited; submit the form again",
			})
		}
		return utils.ErrorHandler(c, err, "Failed to edit server")
	}

	// Handle monitoring service
	if typ != 1 && data.AllowMonitor {
		go func() {
			if err := conn.StartServer(serverId, typ); err != nil {
				log.Println("Failed to start server connection:", err)
			}
		}()
	} else if lastAllowMonitor != data.AllowMonitor && !data.AllowMonitor {
		conn.StopServer(serverId)
	}

	// Log action
	influx.LogAdd(
		tid, uid, "server", "Edit Server: "+data.Name+" (ID"+strconv.FormatInt(serverId, 10)+")",
		c.RealIP(), c.Request().UserAgent(), "medium",
	)
	if hostKeyChanged {
		influx.LogAdd(
			tid, uid, "security", "Confirmed changed SSH host key for Server: "+data.Name+" (ID"+strconv.FormatInt(serverId, 10)+")",
			c.RealIP(), c.Request().UserAgent(), "high",
		)
	}

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Server edited successfully",
	})
}

func sshHostKeyConfirmationRequired(c *echo.Context, validation sshValidationResult) error {
	return c.JSON(409, _type.H{
		Code: "ssh_host_key_confirmation_required",
		Msg:  "Confirm the SSH host key fingerprint before connecting",
		Data: _type.Map{
			"fingerprint": validation.Fingerprint,
			"host_key":    validation.HostKey,
			"changed":     validation.Changed,
		},
	})
}
