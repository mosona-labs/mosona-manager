# Changelog

## v0.1.15 - 2026-08-28

This release fixes a critical authentication gap in the Active Agent
terminal handshake: the agent's key-exchange reply is now signed with
its long-term Ed25519 identity key and pinned by the Hub, so a
man-in-the-middle can no longer impersonate an agent terminal.

### Upgrade Notes

- **Authenticated Active Agent handshake (protocol v2).** The Agent signs
  its key-exchange reply over the full handshake transcript (both ephemeral
  X25519 keys, both nonces, HTTP auth headers, agent UID, and request
  path); the Hub verifies the signature against the pinned agent public key
  and derives session keys bound to the transcript. First contact uses
  trust-on-first-use: compare the agent fingerprint printed in the agent
  log at startup with the fingerprint recorded in the Hub security audit
  log before relying on the connection.
- **Existing agents keep working during the upgrade window.** A database
  migration adds `agents.protocol_version`: rows already pinned move to
  v2, unpinned rows stay on v1 and may use the legacy handshake until the
  agent is upgraded and paired. The legacy path is scheduled for removal
  before v0.1.17 — upgrade agents to close the residual T-01 window.
- **New and reinstalled servers require the new Agent binary.** They
  default to protocol v2 and fail closed against pre-v2 agents.
- **Identity mismatch stops reconnection.** If a pinned key no longer
  matches (for example after an agent reinstall), the Hub stops retrying
  and records a high-severity audit event; reinstall the server to re-pair.
- **Servers with monitoring disabled now pair in the background.** Unpaired
  Active Agent servers run a persistent pairing worker (up to one attempt
  per minute per server) so they become ready before a terminal is opened.

### Security

- Sign and verify the Active Agent `KTAgent` handshake reply, pin the agent
  identity key with an atomic compare-and-set, bind session keys to the
  handshake transcript, and confirm the handshake with mutually verified
  finished messages.
- Apply a 15-second deadline to the whole handshake and fail closed on any
  verification error; once pinned to v2, an agent cannot be downgraded to
  the legacy handshake.
- Validate agent `public_key` values (PEM format and Ed25519 key length)
  during team import, normalizing and re-encoding them before they reach
  the database.

### Fixed

- Stop the Agent process from exiting via `log.Fatalln` when a status
  snapshot or encode fails on an authenticated connection; close the
  connection instead.
- Reconnect the Active Agent monitor when a frame fails decryption instead
  of silently desynchronizing the stream sequence.
- Remove finished connection workers from the pool when they exit so
  stopped or mismatched servers leave no stale entries.
- Stop retry loops when the agent row is missing instead of logging an
  error every minute (applies to SSH servers as well).
- Reuse the existing agent identity when rerunning the installer instead of
  failing on the existing key file.

### Added

- `protocol_version` field in team export/import bundles, with backward
  compatibility for archives produced by older versions. Imports preserve an
  explicit legacy version for unpinned Active Agents during the upgrade window;
  Agents with a pinned identity are always imported as v2.
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
