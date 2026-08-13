# Changelog

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
