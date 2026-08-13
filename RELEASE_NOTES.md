# v0.1.11 — Security & Reliability Audit

> **Tagline:** A focused security hardening and reliability pass across the Hub, Agent, and Web UI — no breaking API removals for the bundled frontend, but a few behavioral changes you should review before upgrading.

---

## ⚠️ Before You Upgrade

This release **re-encrypts stored SSH credentials at startup** (legacy AES-CBC → versioned AES-GCM). Read the upgrade notes before applying to auto-updating or unattended instances.

**Mandatory pre-upgrade steps:**

1. **Back up** the Postgres database **and** the `cfg/key` file.
2. **Do not roll back** the image afterwards — builds prior to v0.1.11 cannot read the new credential format and may crash.
3. Review the full [Upgrade Notes](#-upgrade-notes) below for behavioral changes that may affect your setup.

<details>
<summary><strong>📋 Full Upgrade Notes</strong></summary>

- **No downgrade path.** Stored credentials are re-encrypted at startup from legacy AES-CBC to versioned AES-GCM envelopes. Older builds cannot read the new format.
- **Unreadable credentials are isolated, not fatal.** If a ciphertext can't be decrypted during migration (e.g. the key file was previously regenerated), the Hub logs a `WARNING: skipped unreadable encrypted credential` per row and starts normally instead of crash-looping. Re-enter the affected server's credential in the UI, or restore the original key via `MOSONA_ENCRYPTION_KEY_PATH`.
- **`REDIS_PASS` is now honored.** Older builds only read `REDIS_PASSWORD`. Both are now accepted (`REDIS_PASSWORD` takes precedence; setting both to *different* values refuses to start). If you set `REDIS_PASS` to a non-empty value while Redis runs without a password, clear it first.
- **`SECURE_COOKIES` parsing is strict.** Only the literal `true` / `false` are accepted; anything else (e.g. `1`, `yes`) now aborts startup.
- **Health check semantics changed.** The `health` CLI subcommand (Docker healthcheck) now probes **readiness** of Postgres, Redis, and InfluxDB rather than plain liveness. The container may report `unhealthy` during an InfluxDB outage — `docker compose` won't restart on unhealthy, but external monitors or `depends_on: service_healthy` setups will notice.
- **Admin user API breaking change.** `DELETE /api/admin/user/:id` and demoting another admin now require the actor's `current_password` plus a `confirm=<username>` parameter, returning `401 reauthentication_required` otherwise. Third-party scripts hitting these endpoints must be updated.
- **Legacy team exports require risk confirmation.** Current exports retain confirmed SSH host keys. Older exports without host keys can still be imported after an explicit warning; those SSH hosts are trusted by default until their fingerprints are confirmed.
- **Alert configurations are clamped.** Existing rules outside documented bounds are silently adjusted at upgrade (e.g. `expiry_reminder` capped at 7 days, `for_duration` raised to a 1-minute minimum). No data lost, but alert timing may change.
- **Log queries limited to 365 days.** The logs UI/API now query at most the last year; older data stays untouched in InfluxDB.
- **Public status page rate limits.** Public preview pages now cap bootstrap requests (64/s per IP) and concurrent SSE streams (64 per IP). Visitors behind shared NAT may see HTTP 429 and need to refresh.
- **Compose file changes (manual adoption only).** `deploy/compose.yml` now requires `REDIS_PASSWORD` and defaults `SECURE_COOKIES=true`. If you upgrade via `docker compose pull` you keep your existing file and are unaffected; if you adopt the new file, add `REDIS_PASSWORD` to `.env`.
- **Team owner membership normalized.** A startup migration repairs teams whose owner was missing from the member list or held a non-admin role (possible after historical ownership transfers).

</details>

---

## 🔒 Security Highlights

This release ships a comprehensive security audit. Headline improvements:

- **End-to-end credential encryption** — versioned AES-GCM envelopes bound to record context, with automatic migration of legacy CBC ciphertexts.
- **Hardened master key handling** — fail-closed (no silent regeneration when credentials exist), enforced file permissions/ownership, symlinks rejected.
- **SSH host key pinning** — new/edited servers record and enforce host keys; existing servers keep connecting (`trust_legacy_host_key`) and can be pinned by confirming on edit.
- **Stronger auth & sessions** — team sessions revoked on member removal; revoked team access is rejected instead of silently downgrading to viewer; admin self-deletion / self-demotion / last-admin removal rejected with re-authentication required.
- **OIDC support** with discovery, plus validation of OAuth identity subjects (rejects empty/`0`/whitespace subjects).
- **Resource bounds everywhere** — per-IP / per-team / global SSE limits for public preview streams; request/response size limits; upload limits; HTTP timeouts on the active-agent server.
- **Scoped data access** — server categories, alert upserts, and notification delivery are now scoped to the owning team; category deletion is atomic.
- **Secret redaction** — `smtp_password` and `captcha_secret` are redacted in admin settings responses.

<details>
<summary><strong>🛡️ Full security changelog</strong></summary>

- Encrypt stored credentials at rest with versioned AES-GCM envelopes bound to record context; legacy CBC ciphertexts migrated automatically and legacy decryption refused outside migration.
- Harden master key handling: no silent regeneration when credentials exist (fail-closed with clear recovery), key file permissions/ownership enforced (0600/0700, owner must equal process user, symlinks rejected), atomic key file publication.
- Pin SSH host keys: new/edited servers record and enforce host keys; existing servers marked `trust_legacy_host_key` and kept connecting; pin by confirming on edit.
- Authenticate passive agent public keys and scope agent info/WebSocket endpoints to servers with monitoring enabled.
- Validate OAuth identity subjects (reject empty/`0`/whitespace, quarantine invalid existing bindings), add OIDC support with discovery, revoke auth states of disabled/reconfigured providers via config revisions.
- Revoke team sessions when a member leaves; reject requests with revoked team access instead of silent viewer downgrade.
- Scope server categories, server alert upserts, and notification delivery to owning team; atomic category deletion.
- Preserve team ownership invariants: owners must remain admins, deleting a user who owns teams is restricted (409 with team list) instead of cascading, owner memberships normalized at startup.
- Preserve administrator access: self-deletion, self-demotion, last-admin removal rejected; privileged changes require re-authentication.
- Bound public preview streams with per-IP / per-team / global SSE limits and snapshot-based initial payloads.
- Redact `smtp_password` and `captcha_secret` in admin settings responses.
- Parameterize InfluxDB log filters and validate log category/level against allowlists.
- Publish dynamic settings atomically to remove read/write races.
- Serialize system initialization with a database advisory lock; reject repeat initialization with 409.
- Pin agent install state transitions; validate reinstall operations transactionally.
- Reject invalid monitoring inputs (fixes a panic in system usage collection on hosts without CPU percentage data).
- Harden HTTP handling: request/response size & upload limits, timeouts on the active-agent HTTP server, strict captcha verification, secure cookie propagation.
- Tighten agent install directory security (0700/0600, ownership & symlink checks) and agent-side key parsing.

</details>

---

## ✨ What's New

- **Readiness / liveness health endpoints** — `/health/ready` probes Postgres, Redis, and InfluxDB.
- **Notification target pre-validation** — `POST /api/team/notification/validate` validates targets before saving.
- **OIDC protocol selection** for OAuth providers.
- **Generic webhook notifications** with template allowlist and redirect policy.

---

## 🐛 Fixes

- **Audit log writes are now queued & drained** (bounded queue, graceful shutdown drain) instead of unbounded fire-and-forget goroutines.
- **Cleaner server connection lifecycle** — duplicate monitoring connections replaced, old connections awaited on edit/delete/reinstall, agent connections closed on access revocation.
- **Passive-agent WebSocket shutdown is now permanent** instead of silently reconnecting after server-initiated close.
- Database transactions now roll back on **every** exit path.
- **Auto-renewals catch up** — long-expired auto-renew servers advance to the next future period instead of one period per hour.
- Unified Redis password config (`REDIS_PASSWORD` / `REDIS_PASS`).
- Preserved legacy SSH connectivity during host-key pinning rollout.
- Alert configuration bounds enforced on both write paths and existing data.
- Consistent email sending and base-host / trust-proxy handling.

---

## 🖥️ Web UI

- Validate notification targets before saving (dedicated validation endpoint).
- Password re-authentication dialogs for privileged changes and protected user deletion.
- Team owner role / removal controls disabled in the member editor.
- SSH host key confirmation when adding or editing servers.
- Graceful handling of revoked team sessions (redirect instead of broken state).
- OAuth identity protocol configuration (OAuth 2.0 / OIDC).
- Self-hosted notification targets restored in the channel editor.
- **Edit & delete actions on dashboard server cards**, exposed via a shared context menu with a touch-friendly kebab trigger.
- Avatar source switched from `gravatar.webp.se` → `www.gravatar.com`.

---

**Full changelog:** [`CHANGELOG.md`](./CHANGELOG.md)
