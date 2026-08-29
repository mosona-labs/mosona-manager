package ateam

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mosona-manager/internal/_type"
	"mosona-manager/internal/config"
	"mosona-manager/internal/connect/conn"
	connectSSH "mosona-manager/internal/connect/ssh"
	"mosona-manager/internal/db"
	"mosona-manager/internal/influx"
	"mosona-manager/internal/notification"
	"mosona-manager/internal/security/exportcrypto"
	"mosona-manager/internal/siteaccess"
	"mosona-manager/internal/utils"
	"mosona-manager/internal/utils/encrypt"
	"mosona-manager/pkg/identity"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v5"
	"github.com/pquerna/otp/totp"
)

const teamExportVersion = 1

var errInvalidTeamImport = errors.New("invalid team import")

type teamExportAuthRequest struct {
	TOTPCode       string `json:"totp_code"`
	ExportPassword string `json:"export_password"`
}

type teamImportRequest struct {
	TOTPCode               string                      `json:"totp_code"`
	ExportPassword         string                      `json:"export_password"`
	Encrypted              *exportcrypto.EncryptedFile `json:"encrypted"`
	Data                   json.RawMessage             `json:"data"`
	TrustLegacySSHHostKeys bool                        `json:"trust_legacy_ssh_host_keys"`
}

type teamExportBundle struct {
	Version       int                      `json:"version"`
	ExportedAt    time.Time                `json:"exported_at"`
	Team          teamExportTeam           `json:"team"`
	Categories    []teamExportCategory     `json:"categories"`
	Keys          []teamExportKey          `json:"keys"`
	TeamAlerts    []teamExportAlert        `json:"team_alerts"`
	Notifications []_type.TeamNotification `json:"notifications"`
	PublicPage    *teamExportPublicPage    `json:"public_page,omitempty"`
	Servers       []teamExportServer       `json:"servers"`
}

type teamExportTeam struct {
	Name        string `json:"name" db:"name"`
	Description string `json:"description" db:"description"`
	Color       string `json:"color" db:"color"`
	Image       string `json:"image" db:"image"`
}

type teamExportCategory struct {
	RefID int64  `json:"ref_id" db:"ref_id"`
	Name  string `json:"name" db:"name"`
	Sort  int    `json:"sort" db:"sort"`
}

type teamExportKey struct {
	RefID     int64     `json:"ref_id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	Password  *string   `json:"password,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type teamExportAlert struct {
	Item        string `json:"item" db:"item"`
	Threshold   int    `json:"threshold" db:"threshold"`
	ForDuration int    `json:"for_duration" db:"for_duration"`
}

type teamExportPublicPage struct {
	Enabled     bool    `json:"enabled" db:"enabled"`
	Name        *string `json:"name,omitempty" db:"name"`
	Domain      *string `json:"domain,omitempty" db:"domain"`
	Title       *string `json:"title,omitempty" db:"title"`
	Description *string `json:"description,omitempty" db:"description"`
	CustomCSS   *string `json:"custom_css,omitempty" db:"custom_css"`
}

type teamExportServer struct {
	RefID         int64                   `json:"ref_id" db:"ref_id"`
	CategoryRef   int64                   `json:"category_ref" db:"category_ref"`
	Type          int16                   `json:"type" db:"type"`
	Name          string                  `json:"name" db:"name"`
	AllowMonitor  bool                    `json:"allow_monitor" db:"allow_monitor"`
	AllowTerminal bool                    `json:"allow_terminal" db:"allow_terminal"`
	Weight        int                     `json:"weight" db:"weight"`
	Info          teamExportServerInfo    `json:"info"`
	AdvancedInfo  teamExportServerInfoAdv `json:"advanced_info"`
	SSH           *teamExportSSH          `json:"ssh,omitempty"`
	Agent         *teamExportAgent        `json:"agent,omitempty"`
	EnrollToken   *teamExportEnrollToken  `json:"enroll_token,omitempty"`
	Alerts        []teamExportAlert       `json:"alerts"`
}

type teamExportServerInfo struct {
	OS          *string    `json:"os,omitempty" db:"os"`
	County      *string    `json:"county,omitempty" db:"county"`
	Area        *string    `json:"area,omitempty" db:"area"`
	OpenTime    *time.Time `json:"open_time,omitempty" db:"open_time"`
	Note        *string    `json:"note,omitempty" db:"note"`
	Provider    *string    `json:"provider,omitempty" db:"provider"`
	Cycle       *int       `json:"cycle,omitempty" db:"cycle"`
	StartTime   *time.Time `json:"start_time,omitempty" db:"start_time"`
	EndTime     *time.Time `json:"end_time,omitempty" db:"end_time"`
	Amount      *string    `json:"amount,omitempty" db:"amount"`
	AutoRenew   *bool      `json:"auto_renew,omitempty" db:"auto_renew"`
	Bandwidth   *string    `json:"bandwidth,omitempty" db:"bandwidth"`
	Traffic     *string    `json:"traffic,omitempty" db:"traffic"`
	TrafficType *int       `json:"traffic_type,omitempty" db:"traffic_type"`
	NotePublic  *string    `json:"note_public,omitempty" db:"note_public"`
	Online      bool       `json:"online" db:"online"`
}

type teamExportServerInfoAdv struct {
	Hostname *string `json:"hostname,omitempty" db:"hostname"`
	CPUName  *string `json:"cpu_name,omitempty" db:"cpu_name"`
	CoreC    *int    `json:"core_c,omitempty" db:"core_c"`
	CoreT    *int    `json:"core_t,omitempty" db:"core_t"`
	Kernel   *string `json:"kernel,omitempty" db:"kernel"`
	IP       *string `json:"ip,omitempty" db:"ip"`
	Arch     *string `json:"arch,omitempty" db:"arch"`
}

type teamExportSSH struct {
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	KeyRef   int64  `json:"key_ref"`
	Password string `json:"password"`
	HostKey  string `json:"host_key,omitempty" db:"host_key"`
}

type teamExportAgent struct {
	AgentUID        string     `json:"agent_uid" db:"agent_uid"`
	Status          int16      `json:"status" db:"status"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty" db:"last_seen_at"`
	LastIP          string     `json:"last_ip" db:"last_ip"`
	LastVersion     string     `json:"last_version" db:"last_version"`
	PublicKey       string     `json:"public_key" db:"public_key"`
	PrivateKey      string     `json:"private_key" db:"private_key"`
	ProtocolVersion int16      `json:"protocol_version,omitempty" db:"protocol_version"`
	Host            string     `json:"host" db:"host"`
	Port            int        `json:"port" db:"port"`
}

type teamExportEnrollToken struct {
	TokenHash string    `json:"token_hash" db:"token_hash"`
	IsRevoked bool      `json:"is_revoked" db:"is_revoked"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

func exportTeam(c *echo.Context) error {
	tid, uid, err := requireTeamOwnerAndTOTP(c)
	if err != nil {
		return err
	}
	_ = uid

	var req teamExportAuthRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid request data"})
	}
	if err := requireTOTPCode(c, req.TOTPCode); err != nil {
		return err
	}

	data, err := buildTeamExport(tid)
	if err != nil {
		return utils.ErrorHandler(c, err, "Failed to export team data")
	}
	encrypted, err := exportcrypto.EncryptJSON(req.ExportPassword, data)
	if err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: err.Error()})
	}
	influx.LogAdd(tid, uid, "team", "Export Team Configuration (encrypted)", c.RealIP(), c.Request().UserAgent(), "high")

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team data exported",
		Data: encrypted,
	})
}

func importTeam(c *echo.Context) error {
	tid, _, err := requireTeamOwnerAndTOTP(c)
	if err != nil {
		return err
	}

	var req teamImportRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Invalid request data"})
	}
	if err := requireTOTPCode(c, req.TOTPCode); err != nil {
		return err
	}

	bundle, err := decodeTeamImportBundle(req)
	if err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: err.Error()})
	}
	if bundle.Version != teamExportVersion {
		return c.JSON(400, _type.H{Code: "invalid", Msg: "Unsupported export version"})
	}
	legacySSHServers, err := inspectImportedSSHHostKeys(bundle.Servers)
	if err != nil {
		return c.JSON(400, _type.H{Code: "invalid", Msg: err.Error()})
	}
	if len(legacySSHServers) > 0 && !req.TrustLegacySSHHostKeys {
		return legacySSHHostKeyConfirmationRequired(c, legacySSHServers)
	}

	oldServers, newServers, err := applyTeamImport(tid, bundle, req.TrustLegacySSHHostKeys)
	if err != nil {
		if errors.Is(err, notification.ErrInvalidConfiguration) || errors.Is(err, errInvalidTeamImport) {
			return c.JSON(400, _type.H{Code: "invalid", Msg: err.Error()})
		}
		return utils.ErrorHandler(c, err, "Failed to import team data")
	}
	for _, serverID := range oldServers {
		conn.StopServer(serverID)
	}
	defer func() {
		for _, server := range newServers {
			if reconcileErr := conn.ReconcileServer(server.id); reconcileErr != nil {
				log.Println("Failed to reconcile imported server connection:", reconcileErr)
			}
		}
	}()
	if err = siteaccess.Refresh(); err != nil {
		return utils.ErrorHandler(c, err, "Failed to refresh site access cache")
	}
	for _, serverID := range oldServers {
		if err = influx.RemoveServerStatus(serverID); err != nil {
			return utils.ErrorHandler(c, err, "Failed to clear server status from InfluxDB")
		}
	}
	for _, server := range newServers {
		if err = influx.RemoveServerStatus(server.id); err != nil {
			return utils.ErrorHandler(c, err, "Failed to clear server status from InfluxDB")
		}
	}
	influx.LogAdd(tid, c.Get("uid").(int64), "team", "Import Team Configuration", c.RealIP(), c.Request().UserAgent(), "high")

	return c.JSON(200, _type.H{
		Code: "ok",
		Msg:  "Team data imported",
	})
}

func legacySSHHostKeyConfirmationRequired(c *echo.Context, servers []string) error {
	return c.JSON(409, _type.H{
		Code: "legacy_ssh_host_key_confirmation_required",
		Msg:  "This export contains SSH servers without pinned host keys; confirm that they may be trusted on import",
		Data: _type.Map{
			"count":   len(servers),
			"servers": servers,
		},
	})
}

func decodeTeamImportBundle(req teamImportRequest) (teamExportBundle, error) {
	if hasJSON(req.Data) {
		var bundle teamExportBundle
		if err := json.Unmarshal(req.Data, &bundle); err != nil {
			return bundle, errors.New("import payload is not valid team data")
		}
		return bundle, nil
	}

	var bundle teamExportBundle
	if err := exportcrypto.DecryptJSON(req.ExportPassword, req.Encrypted, &bundle); err != nil {
		return bundle, err
	}
	return bundle, nil
}

func hasJSON(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null"))
}

func requireTeamOwnerAndTOTP(c *echo.Context) (int64, int64, error) {
	tid, _ := c.Get("tid").(int64)
	uid, _ := c.Get("uid").(int64)
	if tid == 0 || uid == 0 {
		err := c.JSON(400, _type.H{Code: "error", Msg: "Invalid team data"})
		return 0, 0, err
	}

	isOwner, err := db.IsTeamOwner(tid, uid)
	if err != nil {
		err = utils.ErrorHandler(c, err, "Database error")
		return 0, 0, err
	}
	if !isOwner {
		err = c.JSON(403, _type.H{Code: "forbidden", Msg: "Only the team owner can import or export team data"})
		return 0, 0, err
	}

	user, err := db.GetUserAuthById(uid)
	if err != nil {
		err = utils.ErrorHandler(c, err, "Failed to get user data")
		return 0, 0, err
	}
	if user.TOTP == nil || *user.TOTP == "" {
		err = c.JSON(403, _type.H{Code: "totp_required", Msg: "TOTP must be enabled before importing or exporting team data"})
		return 0, 0, err
	}
	c.Set("team_export_totp_secret", *user.TOTP)

	return tid, uid, nil
}

func requireTOTPCode(c *echo.Context, code string) error {
	secret, _ := c.Get("team_export_totp_secret").(string)
	if code == "" {
		return c.JSON(401, _type.H{Code: "verify", Msg: "TOTP code required"})
	}
	if secret == "" || !totp.Validate(code, secret) {
		return c.JSON(400, _type.H{Code: "error", Msg: "Invalid TOTP code"})
	}
	return nil
}

func buildTeamExport(teamID int64) (teamExportBundle, error) {
	data := teamExportBundle{
		Version:    teamExportVersion,
		ExportedAt: time.Now().UTC(),
	}

	if err := db.Db.Get(&data.Team, "SELECT name, description, color, image FROM teams WHERE id = $1", teamID); err != nil {
		return data, err
	}
	if err := db.Db.Select(&data.Categories, "SELECT id AS ref_id, name, sort FROM categories WHERE team = $1 ORDER BY sort, id", teamID); err != nil {
		return data, err
	}
	if err := exportKeys(teamID, &data); err != nil {
		return data, err
	}
	if err := db.Db.Select(&data.TeamAlerts, "SELECT item, threshold, for_duration FROM team_alerts WHERE team_id = $1 ORDER BY item", teamID); err != nil {
		return data, err
	}
	if err := db.Db.Select(&data.Notifications, "SELECT module, target FROM teams_notifications WHERE team_id = $1 ORDER BY id", teamID); err != nil {
		return data, err
	}
	if err := exportPublicPage(teamID, &data); err != nil {
		return data, err
	}
	if err := exportServers(teamID, &data); err != nil {
		return data, err
	}

	return data, nil
}

func exportKeys(teamID int64, data *teamExportBundle) error {
	type keyRow struct {
		RefID     int64     `db:"ref_id"`
		Name      string    `db:"name"`
		Content   []byte    `db:"content"`
		Password  []byte    `db:"password"`
		CreatedAt time.Time `db:"created_at"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	var rows []keyRow
	if err := db.Db.Select(&rows, "SELECT id AS ref_id, name, content, password, created_at, updated_at FROM keys WHERE team_id = $1 ORDER BY id", teamID); err != nil {
		return err
	}

	for _, row := range rows {
		content, err := encrypt.Decrypt(row.Content, encrypt.Key, encrypt.KeyContentContext(row.RefID))
		if err != nil {
			return err
		}
		key := teamExportKey{
			RefID:     row.RefID,
			Name:      row.Name,
			Content:   string(content),
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}
		if len(row.Password) > 0 {
			password, err := encrypt.Decrypt(row.Password, encrypt.Key, encrypt.KeyPasswordContext(row.RefID))
			if err != nil {
				return err
			}
			value := string(password)
			key.Password = &value
		}
		data.Keys = append(data.Keys, key)
	}
	return nil
}

func exportPublicPage(teamID int64, data *teamExportBundle) error {
	var page teamExportPublicPage
	err := db.Db.Get(&page, "SELECT enabled, name, domain, title, description, custom_css FROM team_public_pages WHERE team_id = $1", teamID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	data.PublicPage = &page
	return nil
}

func exportServers(teamID int64, data *teamExportBundle) error {
	if err := db.Db.Select(&data.Servers, `
		SELECT id AS ref_id, category AS category_ref, type, name, allow_monitor, allow_terminal, weight
		FROM servers
		WHERE team_id = $1
		ORDER BY id
	`, teamID); err != nil {
		return err
	}

	for i := range data.Servers {
		server := &data.Servers[i]
		if err := db.Db.Get(&server.Info, `
			SELECT os, county, area, open_time, note, provider, cycle, start_time, end_time, amount,
			       auto_renew, bandwidth, traffic, traffic_type, note_public, online
			FROM server_info
			WHERE sid = $1
		`, server.RefID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := db.Db.Get(&server.AdvancedInfo, `
			SELECT hostname, cpu_name, core_c, core_t, kernel, ip, arch
			FROM server_info_adv
			WHERE sid = $1
		`, server.RefID); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := exportServerConnection(server); err != nil {
			return err
		}
		if err := db.Db.Select(&server.Alerts, `
			SELECT item, threshold, for_duration
			FROM server_alerts
			WHERE server_id = $1
			ORDER BY item
		`, server.RefID); err != nil {
			return err
		}
	}
	return nil
}

func exportServerConnection(server *teamExportServer) error {
	switch server.Type {
	case 0:
		type sshRow struct {
			Address  string `db:"address"`
			Port     int    `db:"port"`
			Username string `db:"username"`
			KeyRef   int64  `db:"key_ref"`
			Password []byte `db:"password"`
			HostKey  string `db:"host_key"`
		}
		var row sshRow
		if err := db.Db.Get(&row, "SELECT address, port, username, key_id AS key_ref, password, COALESCE(host_key, '') AS host_key FROM ssh WHERE server_id = $1", server.RefID); err != nil {
			return err
		}
		password, err := encrypt.Decrypt(row.Password, encrypt.Key, encrypt.SSHPasswordContext(server.RefID))
		if err != nil {
			return err
		}
		server.SSH = &teamExportSSH{
			Address:  row.Address,
			Port:     row.Port,
			Username: row.Username,
			KeyRef:   row.KeyRef,
			Password: string(password),
			HostKey:  row.HostKey,
		}
	case 1, 2:
		var agent teamExportAgent
		err := db.Db.Get(&agent, `
			SELECT agent_uid, status, last_seen_at, last_ip, last_version, public_key, private_key, protocol_version, host, port
			FROM agents
			WHERE server_id = $1
		`, server.RefID)
		if err == nil {
			server.Agent = &agent
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		var token teamExportEnrollToken
		err = db.Db.Get(&token, "SELECT token_hash, is_revoked, created_at FROM enroll_tokens WHERE server_id = $1", server.RefID)
		if err == nil {
			server.EnrollToken = &token
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

type importedServerRuntime struct {
	id int64
}

func applyTeamImport(teamID int64, data teamExportBundle, trustLegacySSHHostKeys bool) ([]int64, []importedServerRuntime, error) {
	legacySSHServers, err := inspectImportedSSHHostKeys(data.Servers)
	if err != nil {
		return nil, nil, err
	}
	if len(legacySSHServers) > 0 && !trustLegacySSHHostKeys {
		return nil, nil, fmt.Errorf("%w: importing SSH servers without host keys requires explicit confirmation", errInvalidTeamImport)
	}
	data.Servers, err = normalizeImportedAgentPublicKeys(data.Servers)
	if err != nil {
		return nil, nil, err
	}
	notifications, err := notification.NormalizeEntries(context.Background(), data.Notifications)
	if err != nil {
		return nil, nil, err
	}
	data.Notifications = notifications

	tx, err := db.Db.Beginx()
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var oldServerIDs []int64
	if err = tx.Select(&oldServerIDs, "SELECT id FROM servers WHERE team_id = $1", teamID); err != nil {
		return nil, nil, err
	}

	if _, err = tx.Exec(
		"UPDATE teams SET name = $1, description = $2, color = $3, image = $4, updated_at = now() WHERE id = $5",
		data.Team.Name, data.Team.Description, data.Team.Color, data.Team.Image, teamID,
	); err != nil {
		return nil, nil, err
	}

	if err = clearTeamConfig(tx, teamID); err != nil {
		return nil, nil, err
	}

	keyMap, err := importKeys(tx, teamID, data.Keys)
	if err != nil {
		return nil, nil, err
	}
	categoryMap, err := importCategories(tx, teamID, data.Categories)
	if err != nil {
		return nil, nil, err
	}
	if err = importTeamAlerts(tx, teamID, data.TeamAlerts); err != nil {
		return nil, nil, err
	}
	if err = insertNotifications(tx, teamID, data.Notifications); err != nil {
		return nil, nil, err
	}
	if err = importPublicPage(tx, teamID, data.PublicPage); err != nil {
		return nil, nil, err
	}
	newServers, err := importServers(tx, teamID, data.Servers, categoryMap, keyMap, trustLegacySSHHostKeys)
	if err != nil {
		return nil, nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, nil, err
	}

	return oldServerIDs, newServers, nil
}

func normalizeImportedAgentPublicKeys(servers []teamExportServer) ([]teamExportServer, error) {
	normalized := append([]teamExportServer(nil), servers...)
	for i := range normalized {
		if normalized[i].Agent == nil {
			continue
		}
		agent := *normalized[i].Agent
		raw := strings.TrimSpace(agent.PublicKey)
		if raw == "" {
			agent.PublicKey = ""
			normalized[i].Agent = &agent
			continue
		}
		publicKey, err := identity.ParseEd25519PublicKeyPEM([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Agent public key for server %q: %v", errInvalidTeamImport, normalized[i].Name, err)
		}
		agent.PublicKey, err = identity.EncodeEd25519PublicKeyPEM(publicKey)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid Agent public key for server %q: %v", errInvalidTeamImport, normalized[i].Name, err)
		}
		normalized[i].Agent = &agent
	}
	return normalized, nil
}

func inspectImportedSSHHostKeys(servers []teamExportServer) ([]string, error) {
	var legacyServers []string
	for _, server := range servers {
		if server.Type != 0 {
			continue
		}
		if server.SSH == nil {
			return nil, fmt.Errorf("%w: SSH server %q is missing SSH configuration", errInvalidTeamImport, server.Name)
		}
		if server.SSH.HostKey == "" {
			legacyServers = append(legacyServers, server.Name)
			continue
		}
		if _, err := connectSSH.NormalizeHostKey(server.SSH.HostKey); err != nil {
			return nil, fmt.Errorf("%w: invalid SSH host key for server %q: %v", errInvalidTeamImport, server.Name, err)
		}
	}
	return legacyServers, nil
}

func clearTeamConfig(tx *sqlx.Tx, teamID int64) error {
	if _, err := tx.Exec("DELETE FROM servers WHERE team_id = $1", teamID); err != nil {
		return err
	}
	for _, stmt := range []string{
		"DELETE FROM keys WHERE team_id = $1",
		"DELETE FROM categories WHERE team = $1",
		"DELETE FROM team_alerts WHERE team_id = $1",
		"DELETE FROM teams_notifications WHERE team_id = $1",
		"DELETE FROM team_public_pages WHERE team_id = $1",
	} {
		if _, err := tx.Exec(stmt, teamID); err != nil {
			return err
		}
	}
	return nil
}

func importKeys(tx *sqlx.Tx, teamID int64, keys []teamExportKey) (map[int64]int64, error) {
	keyMap := make(map[int64]int64, len(keys))
	for _, key := range keys {
		var newID int64
		if err := tx.QueryRow(
			"INSERT INTO keys (team_id, name, content, password) VALUES ($1, $2, $3, NULL) RETURNING id",
			teamID, key.Name, []byte{},
		).Scan(&newID); err != nil {
			return nil, err
		}
		content, err := encrypt.Encrypt([]byte(key.Content), encrypt.Key, encrypt.KeyContentContext(newID))
		if err != nil {
			return nil, err
		}
		var password []byte
		if key.Password != nil {
			password, err = encrypt.Encrypt([]byte(*key.Password), encrypt.Key, encrypt.KeyPasswordContext(newID))
			if err != nil {
				return nil, err
			}
		}
		var storedPassword any
		if len(password) > 0 {
			storedPassword = password
		}
		if _, err = tx.Exec("UPDATE keys SET content = $1, password = $2 WHERE id = $3", content, storedPassword, newID); err != nil {
			return nil, err
		}
		keyMap[key.RefID] = newID
	}
	return keyMap, nil
}

func importCategories(tx *sqlx.Tx, teamID int64, categories []teamExportCategory) (map[int64]int64, error) {
	categoryMap := make(map[int64]int64, len(categories))
	if len(categories) == 0 {
		categories = []teamExportCategory{{RefID: 0, Name: "Default"}}
	}
	for _, category := range categories {
		var newID int64
		if err := tx.QueryRow(
			"INSERT INTO categories (team, name, sort) VALUES ($1, $2, $3) RETURNING id",
			teamID, category.Name, category.Sort,
		).Scan(&newID); err != nil {
			return nil, err
		}
		categoryMap[category.RefID] = newID
	}
	return categoryMap, nil
}

func importTeamAlerts(tx *sqlx.Tx, teamID int64, alerts []teamExportAlert) error {
	for _, alert := range alerts {
		if _, err := tx.Exec(
			"INSERT INTO team_alerts (team_id, item, threshold, for_duration) VALUES ($1, $2, $3, $4)",
			teamID, alert.Item, alert.Threshold, alert.ForDuration,
		); err != nil {
			return err
		}
	}
	return nil
}

func importNotifications(tx *sqlx.Tx, teamID int64, notifications []_type.TeamNotification) error {
	normalized, err := notification.NormalizeEntries(context.Background(), notifications)
	if err != nil {
		return err
	}
	return insertNotifications(tx, teamID, normalized)
}

func insertNotifications(tx *sqlx.Tx, teamID int64, notifications []_type.TeamNotification) error {
	for _, item := range notifications {
		if _, err := tx.Exec(
			"INSERT INTO teams_notifications (team_id, module, target) VALUES ($1, $2, $3)",
			teamID, item.Module, item.Target,
		); err != nil {
			return err
		}
	}
	return nil
}

func importPublicPage(tx *sqlx.Tx, teamID int64, page *teamExportPublicPage) error {
	if page == nil {
		return nil
	}

	name, err := normalizePublicPageName(exportStr(page.Name))
	if err != nil {
		return fmt.Errorf("invalid public page name: %w", err)
	}
	domain, err := normalizePublicPageDomain(exportStr(page.Domain))
	if err != nil {
		return fmt.Errorf("invalid public page domain: %w", err)
	}
	title, err := normalizePublicPageTitle(exportStr(page.Title))
	if err != nil {
		return fmt.Errorf("invalid public page title: %w", err)
	}
	description := normalizePublicPageDescription(exportStr(page.Description))
	customCSS := normalizePublicPageCustomCSS(exportStr(page.CustomCSS))

	if page.Enabled && name == nil && domain == nil {
		return errors.New("public page enabled requires name or domain")
	}
	if domain != nil {
		baseDomain := normalizeConfiguredBaseDomain(config.ReadDynamicConf().Domain)
		if baseDomain != "" && *domain == baseDomain {
			return errors.New("public page domain cannot match application base domain")
		}
	}

	_, err = tx.Exec(
		`INSERT INTO team_public_pages (team_id, enabled, name, domain, title, description, custom_css, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		teamID, page.Enabled, name, domain, title, description, customCSS,
	)
	return err
}

func exportStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func importServers(tx *sqlx.Tx, teamID int64, servers []teamExportServer, categoryMap, keyMap map[int64]int64, trustLegacySSHHostKeys bool) ([]importedServerRuntime, error) {
	var defaultCategoryID int64
	for _, id := range categoryMap {
		defaultCategoryID = id
		break
	}
	if defaultCategoryID == 0 {
		return nil, fmt.Errorf("no category available for imported servers")
	}

	imported := make([]importedServerRuntime, 0, len(servers))
	for _, server := range servers {
		categoryID := categoryMap[server.CategoryRef]
		if categoryID == 0 {
			categoryID = defaultCategoryID
		}

		var newServerID int64
		if err := tx.QueryRow(
			`INSERT INTO servers (team_id, name, type, category, allow_monitor, allow_terminal, weight)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			teamID, server.Name, server.Type, categoryID, server.AllowMonitor, server.AllowTerminal, server.Weight,
		).Scan(&newServerID); err != nil {
			return nil, err
		}
		if err := importServerInfo(tx, newServerID, server.Type, server.Info); err != nil {
			return nil, err
		}
		if err := importServerAdvancedInfo(tx, newServerID, server.Type, server.AdvancedInfo); err != nil {
			return nil, err
		}
		if err := importServerConnection(tx, newServerID, server, keyMap, trustLegacySSHHostKeys); err != nil {
			return nil, err
		}
		for _, alert := range server.Alerts {
			if _, err := tx.Exec(
				"INSERT INTO server_alerts (server_id, item, threshold, for_duration) VALUES ($1, $2, $3, $4)",
				newServerID, alert.Item, alert.Threshold, alert.ForDuration,
			); err != nil {
				return nil, err
			}
		}
		imported = append(imported, importedServerRuntime{
			id: newServerID,
		})
	}
	return imported, nil
}

func importServerInfo(tx *sqlx.Tx, serverID int64, serverType int16, info teamExportServerInfo) error {
	info = normalizeImportedServerInfo(serverType, info)
	_, err := tx.Exec(
		`INSERT INTO server_info (
			sid, os, county, area, open_time, note, provider, cycle, start_time, end_time, amount,
			auto_renew, bandwidth, traffic, traffic_type, note_public, online
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		serverID, info.OS, info.County, info.Area, info.OpenTime, info.Note, info.Provider, info.Cycle,
		info.StartTime, info.EndTime, info.Amount, info.AutoRenew, info.Bandwidth, info.Traffic,
		info.TrafficType, info.NotePublic, info.Online,
	)
	return err
}

func normalizeImportedServerInfo(serverType int16, info teamExportServerInfo) teamExportServerInfo {
	if serverType == 2 {
		info.OS = nil
		info.County = nil
		info.Area = nil
		info.OpenTime = nil
		info.Online = false
	}
	return info
}

func importServerAdvancedInfo(tx *sqlx.Tx, serverID int64, serverType int16, info teamExportServerInfoAdv) error {
	info = normalizeImportedServerAdvancedInfo(serverType, info)
	_, err := tx.Exec(
		`INSERT INTO server_info_adv (sid, hostname, cpu_name, core_c, core_t, kernel, ip, arch)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		serverID, info.Hostname, info.CPUName, info.CoreC, info.CoreT, info.Kernel, info.IP, info.Arch,
	)
	return err
}

func normalizeImportedServerAdvancedInfo(serverType int16, info teamExportServerInfoAdv) teamExportServerInfoAdv {
	if serverType == 2 {
		return teamExportServerInfoAdv{}
	}
	return info
}

func importServerConnection(tx *sqlx.Tx, serverID int64, server teamExportServer, keyMap map[int64]int64, trustLegacySSHHostKeys bool) error {
	switch server.Type {
	case 0:
		if server.SSH == nil {
			return fmt.Errorf("%w: SSH server %q is missing SSH configuration", errInvalidTeamImport, server.Name)
		}
		if server.SSH.HostKey == "" && !trustLegacySSHHostKeys {
			return fmt.Errorf("%w: importing SSH server %q without a host key requires explicit confirmation", errInvalidTeamImport, server.Name)
		}
		keyID := keyMap[server.SSH.KeyRef]
		password, err := encrypt.Encrypt([]byte(server.SSH.Password), encrypt.Key, encrypt.SSHPasswordContext(serverID))
		if err != nil {
			return err
		}
		var hostKey any
		trustLegacyHostKey := server.SSH.HostKey == ""
		if !trustLegacyHostKey {
			hostKey, err = connectSSH.NormalizeHostKey(server.SSH.HostKey)
			if err != nil {
				return fmt.Errorf("%w: invalid SSH host key for server %q: %v", errInvalidTeamImport, server.Name, err)
			}
		}
		_, err = tx.Exec(
			"INSERT INTO ssh (server_id, address, port, username, key_id, password, host_key, trust_legacy_host_key) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
			serverID, server.SSH.Address, server.SSH.Port, server.SSH.Username, keyID, password, hostKey, trustLegacyHostKey,
		)
		return err
	case 1, 2:
		if server.Agent != nil {
			lastSeenAt, lastIP, lastVersion := normalizeImportedAgentRuntime(server.Type, server.Agent)
			if _, err := tx.Exec(
				`INSERT INTO agents (server_id, agent_uid, status, last_seen_at, last_ip, last_version, public_key, private_key, protocol_version, host, port)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				serverID, server.Agent.AgentUID, server.Agent.Status, lastSeenAt, lastIP,
				lastVersion, server.Agent.PublicKey, server.Agent.PrivateKey, normalizeImportedAgentProtocol(server.Type, server.Agent), server.Agent.Host, server.Agent.Port,
			); err != nil {
				return err
			}
		}
		if server.EnrollToken != nil {
			if _, err := tx.Exec(
				"INSERT INTO enroll_tokens (server_id, token_hash, is_revoked, created_at) VALUES ($1, $2, $3, $4)",
				serverID, server.EnrollToken.TokenHash, server.EnrollToken.IsRevoked, server.EnrollToken.CreatedAt,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeImportedAgentProtocol(serverType int16, agent *teamExportAgent) int16 {
	if strings.TrimSpace(agent.PublicKey) != "" {
		return 2
	}
	if serverType == 1 && (agent.ProtocolVersion == 0 || agent.ProtocolVersion == 1) {
		return 1
	}
	return 2
}

func normalizeImportedAgentRuntime(serverType int16, agent *teamExportAgent) (*time.Time, string, string) {
	if serverType == 2 {
		return nil, "", ""
	}
	if agent.LastSeenAt == nil {
		now := time.Now()
		return &now, agent.LastIP, agent.LastVersion
	}
	return agent.LastSeenAt, agent.LastIP, agent.LastVersion
}
