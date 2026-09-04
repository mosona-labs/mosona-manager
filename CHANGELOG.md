# Changelog

## v0.1.16 (2026-9-4)

### Fixed

- Fix server alert notifications never firing: the batched InfluxDB alert
  queries wrapped a single duration group in `union(tables: [branch0])`,
  which Flux rejects ("union must have at least two streams as input"), so
  every alert evaluation cycle skipped all rules whenever a team's rules for
  an item shared one lookback duration (the default setup). Single-group
  queries now emit the branch pipeline directly.
- Keep the valid disk rows when `df` partially fails during SSH status
  collection (for example a stale FUSE mount makes `df` list every
  filesystem and then exit non-zero): the disk sample now uses the rows
  that parsed instead of discarding the whole snapshot, and only falls
  back to the last cached sample when `df` times out or produces no valid
  rows at all.

### Web UI

- Some small design changes.

## v0.1.15 (2026-8-30)

This release authenticates the Active Agent terminal handshake (protocol v2:
the reply is signed with the agent's Ed25519 identity key and pinned by the
Hub, so a man-in-the-middle can no longer impersonate an agent terminal) and
hardens Passive terminal sessions (bound to their server, claimed atomically,
session IDs moved out of URLs).

### Upgrade Notes

- **No downgrade path.** A startup migration encrypts existing
  `agents.private_key` values with the master key (Agents need no reinstall
  or re-pairing) and adds `agents.protocol_version` and
  `servers.public_visible`. Back up PostgreSQL and `cfg/key` together; an
  older Hub run against the migrated database fails every Active Agent
  connection and terminal.
- **Upgrade Active Agents before v0.1.17.** Pinned agents move to protocol
  v2; unpinned ones keep the legacy handshake until upgraded and paired,
  and that legacy path is removed before v0.1.17 (close the residual
  man-in-the-middle window). New and reinstalled servers require the new
  Agent binary, default to v2, and fail closed against pre-v2 agents.
- **First contact is trust-on-first-use.** Compare the agent fingerprint
  printed in the agent log with the one recorded in the Hub security audit
  log before relying on the connection. If a pinned key later stops
  matching (e.g. after an agent reinstall), the Hub stops retrying, records
  a high-severity audit event, and the server must be reinstalled to
  re-pair. Unpaired Active Agent servers pair in the background (about one
  attempt per minute) so a terminal is ready when opened.
- **Upgrade Passive Agents before v0.1.17.** Current Agents send the
  terminal session ID in `X-Agent-Terminal-Session`; the legacy
  `/api/agent/terminal/:session_id` route remains available through
  v0.1.16. New Passive Agents fall back to it on older Hubs, so Hub and
  Agent upgrade order is independent — legacy Hubs still log the session
  ID until upgraded.
- **Passive terminal disconnects end the session.** Session IDs are
  single-use and are not retried; the browser reconnect (automatic or via
  Reconnect) opens a new PTY. Scrollback is kept; remote programs are not
  resumed.
- **`servers.public_visible` defaults to visible.** The public status page
  filters on it; editing a server without sending the field (older UIs,
  still-open dashboards, third-party clients) keeps the stored value —
  only an explicit `false` hides a server.

### Security

- Encrypt Active Agent long-term private keys at rest with the versioned
  AES-GCM envelope bound to the Server record; runtime connections and
  exports reject plaintext or ciphertext copied from another Server.
- Enforce uniqueness for non-empty Agent UIDs so an authenticated Passive
  Agent identity resolves to exactly one Server; upgrades fail closed on
  duplicates, and team imports preflight conflicts with a Server-specific
  409 instead of an opaque database error.
- Identify the affected Server or shared SSH Key when a team export hits an
  unreadable credential: exports skip unreadable Servers and Servers that
  depend on a skipped Key by default and record them in the response and
  encrypted bundle; `skip_unreadable_servers: false` requests a strict,
  all-or-nothing export.
- Bind Passive terminal sessions to their target server, consume them
  atomically on first claim, generate identifiers as random UUID v4, and
  require terminal-enabled installed Agents, preventing cross-team or
  repeated claims; keep session identifiers out of URLs on the new
  endpoint, reducing reverse-proxy and WAF log exposure.
- Sign and verify the Active Agent handshake reply over the full transcript,
  pin the identity key with an atomic compare-and-set, bind session keys to
  the transcript, confirm with mutually verified finished messages, apply a
  15-second deadline, fail closed on any verification error, and refuse
  downgrade to the legacy handshake once pinned to v2.
- Validate agent `public_key` values (PEM format and Ed25519 key length)
  during team import, normalizing and re-encoding them before storage.

### Fixed

- Use random UUID v4 identities and fail closed if generation fails during
  Passive Agent enrollment or Active Server creation, instead of risking a
  predictable or all-zero Agent identity.
- Stop the Agent process from exiting via `log.Fatalln` when a status
  snapshot or encode fails on an authenticated connection; close the
  connection instead, and reconnect the monitor when a frame fails
  decryption instead of silently desynchronizing the stream.
- Remove finished connection workers from the pool so stopped or mismatched
  servers leave no stale entries, stop retry loops when the agent row is
  missing instead of logging an error every minute (SSH servers included),
  and reuse the existing agent identity when rerunning the installer.

### Added

- Optional per-server `public_visible` flag (default visible) that hides a
  server from the public status page only — team dashboards, monitoring,
  and terminals are untouched. Set it on add/edit; team export/import
  bundles carry it, and older bundles without the field import as visible.
- `protocol_version` field in team export/import bundles, backward
  compatible with older archives: unpinned Active Agents keep an explicit
  legacy version during the upgrade window, pinned identities import as v2.
- `docs/testing.md` describing the environment variables needed to run the
  test suite.

## v0.1.14 - 2026-08-22

This release reduces Hub load from live monitoring and alerting on larger
fleets, replaces log offset pagination with cursor-based time-range queries,
and hardens admin list search plus static-path joining.

### Upgrade Notes

- **Logs API no longer supports offset pagination.** `GET /api/v1/logs` and
  `GET /api/admin/logs` reject `page` greater than 1 with HTTP 400 (`Offset
  pagination is no longer supported`). Responses return `next_cursor` and
  `has_more` instead of `total`. The bundled web UI is updated; third-party
  clients must follow `next_cursor` (omit it for the newest page). `page=1`
  remains accepted as an alias for the first page.
- **Log queries default to the last 30 days.** Callers may pass RFC3339
  `start` and `end` (maximum span 365 days, unchanged). Message search is
  limited to a 30-day window; a longer range with `message` set returns 400.
  Older points remain in InfluxDB untouched.

### Added

- Optional RFC3339 `start`/`end` filters on team and admin log list
  endpoints, with cursor-based paging (`cursor`, `next_cursor`, `has_more`).

### Performance

- Share one monitor snapshot per team across SSE subscribers (refresh every
  3s, 8s query timeout, 64 concurrent Influx loads) instead of querying
  InfluxDB once per open dashboard.
- Queue and batch server-status writes to InfluxDB (10k-point queue,
  500-point batches, 1s flush), retry a failed batch once, drop the oldest
  points on overflow, and drain the queue on shutdown alongside audit logs.
- Batch alert observation queries (64 servers at a time, grouped by metric
  and `for_duration`) and pre-aggregate windows instead of one Influx query
  per server/rule.
- Parallelize admin-dashboard Influx queries (record counts and system
  usage) with a 15s context timeout.

### Fixed

- Reject parent path segments (`..`) in `SafeJoinUnderRoot`, including
  backslash-separated paths, so static-file joins cannot walk via traversal
  segments that previously cleaned back under the root.
- Validate admin list pagination (`page`/`size`) against shared bounds
  (default 1/20, max 100000/1000) and return 400 on invalid values instead
  of coercing them.
- Escape `LIKE` wildcards in admin user/team search, match numeric IDs
  exactly, and list teams without requiring a member join so empty teams
  appear in the admin list.

### Web UI

- Virtualize dashboard server cards and throttle SSE snapshot commits so
  large fleets stay interactive while status updates stream.
- Replace log page numbers with previous/next cursor paging and a time-range
  selector (24h / 7d / 30d / 90d / 365d); message search clamps the range to
  30 days.
- Render agent-mode servers on the terminal page when SSH/OS fields are
  absent instead of throwing on null address/username.
- Correct monitor chart downsampling so longer ranges (7d / 30d / 180d /
  365d) keep a sensible number of points instead of collapsing or
  over-aggregating.

## v0.1.13 - 2026-08-17

### Fixed

- Distinguish users without an active team (`409 team_required`) from revoked
  team access, and defer team-scoped web UI requests until a team is active,
  preventing new instances from refresh-looping between `/` and
  `/create-team`.
- Stop passive-agent WebSocket reconnect attempts from submitting a full host
  information report before every retry. Startup and jittered periodic reports
  remain unchanged.
- Avoid rewriting unchanged server inventory and alert state rows, preventing
  dead-tuple growth during stable operation.
- Bound agent-information and alert update transactions, and keep agent
  connection shutdown outside the reinstall database transaction.

### Operations

- Label Hub PostgreSQL sessions with `application_name=mosona-manager-hub` and
  default `POSTGRES_IDLE_IN_TRANSACTION_TIMEOUT` to `60s` (`0` disables it).
- Add a [PostgreSQL bloat recovery runbook](./docs/postgres-bloat-recovery.md)
  for diagnosing stale transactions and reclaiming affected tables safely.

## v0.1.12 - 2026-08-14

This release is a comprehensive security and logic audit of the hub, the agent,
and the web UI. It contains no schema-incompatible API removals for the web
frontend (frontend and backend ship in the same image), but several behavioral
changes listed below under **Upgrade Notes**.

### Upgrade Notes

Read these before upgrading auto-updating or unattended instances.

- **No downgrade path.** Stored credentials (SSH passwords/keys) are
  re-encrypted at startup from legacy AES-CBC to versioned AES-GCM envelopes.
  Builds prior to v0.1.11 cannot read the new format and may crash attempting
  to. Back up the Postgres database and the `cfg/key` file before upgrading;
  do not roll back the image afterwards.
- **Credential migration isolates unreadable rows.** If an existing ciphertext
  cannot be decrypted during the startup migration (e.g. the key file was
  previously regenerated), the hub now logs a `WARNING: skipped unreadable
  encrypted credential` per row and starts normally instead of crash-looping.
  The affected server's credential stays unusable until it is re-entered in
  the UI or the original key is restored via `MOSONA_ENCRYPTION_KEY_PATH`.
- **`REDIS_PASS` is now honored.** Older builds only read `REDIS_PASSWORD`;
  the legacy `REDIS_PASS` variable was silently ignored. Both are now accepted
  (`REDIS_PASSWORD` takes precedence; setting both to different values refuses
  to start). If you previously set `REDIS_PASS` to a non-empty value while
  Redis runs without a password, clear it before upgrading or sessions/login
  will fail.
- **`SECURE_COOKIES` parsing is strict.** Only the literal values `true` and
  `false` are accepted; any other value (e.g. `1`, `TRUE`, `yes`) now aborts
  startup instead of being silently treated as `false`.
- **Health check semantics changed.** The `health` CLI subcommand (used by the
  Docker healthcheck) now probes readiness of Postgres, Redis, and InfluxDB
  instead of plain liveness. The container may report `unhealthy` during an
  InfluxDB outage; `docker compose` does not restart containers on unhealthy,
  but external monitoring/`depends_on: service_healthy` setups will notice.
- **Admin user API breaking change.** `DELETE /api/admin/user/:id` and demoting
  another administrator now require the actor's `current_password` and a
  `confirm=<username>` parameter, returning 401 `reauthentication_required`
  otherwise. The bundled web UI is updated; third-party scripts calling these
  endpoints must be updated.
- **Legacy team exports require risk confirmation.** Current exports retain
  confirmed SSH host keys. Older exports without host keys can still be
  imported after an explicit warning; those SSH hosts are trusted by default
  until their fingerprints are confirmed by editing each server.
- **Alert configurations are clamped.** Existing alert rules outside the
  documented bounds are silently adjusted at upgrade (e.g. `expiry_reminder`
  threshold capped at 7 days, `for_duration` raised to a 1-minute minimum,
  zero/negative thresholds corrected). No data is lost, but alert timing may
  change.
- **Log queries limited to 365 days.** The logs UI/API now query at most the
  last year; older data remains in InfluxDB untouched.
- **Public status page rate limits.** Public preview pages now limit bootstrap
  requests (64/s per IP) and concurrent SSE streams (64 per IP). Visitors behind a shared NAT may see HTTP 429 and need to
  refresh.
- **Compose file changes (manual adoption only).** `deploy/compose.yml` now
  requires `REDIS_PASSWORD` and defaults `SECURE_COOKIES=true`. Instances
  upgrading via `docker compose pull` keep their existing file and are
  unaffected; if you adopt the new compose file, add `REDIS_PASSWORD` to your
  `.env`.
- **Team owner membership normalized.** A startup migration repairs teams
  whose owner was missing from the member list or held a non-administrator
  role (possible after historical ownership transfers); such teams are
  editable again and the owner is restored as administrator.

### Security

- Encrypt stored credentials at rest with versioned AES-GCM envelopes bound to
  their record context; legacy CBC ciphertexts are migrated automatically at
  startup and legacy decryption is refused outside the migration.
- Harden master key handling: no silent regeneration when credentials exist
  (fail-closed with a clear recovery message), key file permissions/ownership
  enforced (0600/0700, owner must equal the process user, symlinks rejected),
  atomic key file publication.
- Pin SSH host keys: newly added or edited servers record and enforce the
  host key; existing servers are automatically marked `trust_legacy_host_key`
  and keep connecting, and can be pinned by confirming the key on edit.
- Authenticate passive agent public keys and scope agent info/WebSocket
  endpoints to servers with monitoring enabled.
- Validate OAuth identity subjects (reject empty/`0`/whitespace subjects,
  quarantine invalid existing bindings), add OIDC protocol support with
  discovery, and revoke authorization states of disabled or reconfigured
  providers via config revisions.
- Revoke team sessions when a member is removed or leaves, and reject requests
  with revoked team access instead of silently downgrading to viewer.
- Scope server categories, server alert upserts, and notification delivery to
  their owning team; make category deletion atomic.
- Preserve team ownership invariants: owners must remain administrators,
  deleting a user who owns teams is restricted (409 with the team list)
  instead of cascading, and owner memberships are normalized at startup.
- Preserve administrator access: self-deletion, self-demotion, and removal of
  the last administrator are rejected; privileged user changes require
  re-authentication.
- Bound public preview streams with per-IP/per-team/global SSE limits and
  snapshot-based initial payloads.
- Redact configured secrets (`smtp_password`, `captcha_secret`) in admin
  settings responses.
- Parameterize InfluxDB log filters and validate log category/level against
  allowlists.
- Publish dynamic settings atomically to remove read/write races.
- Serialize system initialization with a database advisory lock and reject
  repeat initialization with 409.
- Pin agent install state transitions and validate reinstall operations
  transactionally.
- Reject invalid monitoring inputs (fixes a panic in system usage collection
  on hosts without CPU percentage data).
- Harden HTTP handling: request/response size and upload limits, timeouts on
  the active-agent HTTP server, strict captcha verification, and secure cookie
  configuration propagation.
- Tighten agent install directory security (permissions repaired to 0700/0600,
  ownership and symlink checks) and agent-side key parsing.

### Added

- Readiness/liveness health endpoints (`/health/ready`) probing Postgres,
  Redis, and InfluxDB.
- `POST /api/team/notification/validate` to pre-validate notification targets
  before saving.
- OIDC protocol selection for OAuth providers.
- Generic notification webhook support with template allowlist and
  redirect policy.

### Fixed

- Queue and drain audit log writes to InfluxDB (bounded queue, graceful
  shutdown drain) instead of unbounded fire-and-forget goroutines.
- Reconcile server connection lifecycle: replace duplicate monitoring
  connections, wait for old connections to stop on edit/delete/reinstall, and
  close agent connections when access is revoked.
- Make passive-agent WebSocket shutdown permanent instead of silently
  reconnecting after server-initiated close.
- Roll back database transactions on every exit path.
- Catch up automatic server renewals: long-expired auto-renew servers are
  advanced from their original anchor to the next future period instead of
  one period per hour.
- Unify Redis password configuration (`REDIS_PASSWORD`/`REDIS_PASS`).
- Preserve legacy SSH server connectivity during host-key pinning rollout.
- Safely replace public preview build artifacts (no symlink following,
  atomic replacement).
- Enforce alert configuration bounds on both write paths and existing data.
- Fix email sending and base-host/trust-proxy handling consistency.

### Web UI

- Validate notification targets before saving, with a dedicated validation
  endpoint integration.
- Confirm privileged user changes and protected user deletion with password
  re-authentication dialogs.
- Protect team owner membership: the owner's role and removal controls are
  disabled in the member editor.
- Confirm SSH host keys when adding or editing servers.
- Handle revoked team sessions gracefully (redirect instead of broken state).
- Support configuring OAuth identity protocols (OAuth 2.0 / OIDC).
- Restore self-hosted notification target options in the channel editor.
- Add edit & delete actions to dashboard server cards, exposed via a shared
  context menu with a touch-friendly kebab trigger.
- Replace `gravatar.webp.se` with `www.gravatar.com` in all components.
