# PostgreSQL Bloat Recovery

Use this runbook when PostgreSQL CPU remains high and small Mosona Manager
tables contain a large number of dead tuples. A long-lived transaction can pin
an old snapshot while periodic updates continue creating obsolete row versions.

## Before recovery

1. Back up PostgreSQL and confirm that the backup can be restored.
2. Deploy a fixed Mosona Manager build before cleaning the tables, then restart
   the Hub so connections from the old build are gone.
3. Use a PostgreSQL superuser only for the diagnostic and maintenance commands
   that require it. Do not terminate a backend until its owner is confirmed.

New Hub connections use `application_name=mosona-manager-hub` and default
`idle_in_transaction_session_timeout` to 60 seconds. Override this with
`POSTGRES_IDLE_IN_TRANSACTION_TIMEOUT` using Go duration syntax (`2m`, `90s`),
or set it to `0` only when equivalent database-side protection exists.

## Identify the blocking transaction

Run this in the affected database:

```sql
SELECT pid,
       usename,
       application_name,
       client_addr,
       state,
       xact_start,
       age(clock_timestamp(), xact_start) AS transaction_age,
       backend_xmin,
       wait_event_type,
       wait_event,
       left(query, 500) AS last_query
FROM pg_stat_activity
WHERE datname = current_database()
  AND state LIKE 'idle in transaction%'
  AND pid <> pg_backend_pid()
ORDER BY xact_start;
```

Confirm the application name, database role, client address, transaction age,
and last query. Older Mosona Manager builds did not set `application_name`, so
an empty value alone neither confirms nor excludes the Hub.

Terminate only a PID that has been positively identified:

```sql
SELECT pg_terminate_backend(12345); -- replace 12345 with the confirmed PID
```

Repeat the diagnostic query and confirm that no unexpected old transaction or
`backend_xmin` remains.

For defense beyond the application connection string, the database owner can
set the timeout for the application role. Replace both identifiers first:

```sql
ALTER ROLE mm_user IN DATABASE mm_db
SET idle_in_transaction_session_timeout = '60s';
```

The role setting applies to new sessions. Reconnect the Hub after changing it.

## Reclaim dead tuples

First run regular VACUUM outside any explicit transaction. It makes space
reusable without taking the long exclusive locks required by `VACUUM FULL`:

```sql
VACUUM (VERBOSE, ANALYZE) agents;
VACUUM (VERBOSE, ANALYZE) server_info;
VACUUM (VERBOSE, ANALYZE) server_info_adv;
VACUUM (VERBOSE, ANALYZE) server_alerts;
```

Check the result:

```sql
SELECT relname,
       n_live_tup,
       n_dead_tup,
       last_autovacuum,
       pg_size_pretty(pg_total_relation_size(relid)) AS total_size
FROM pg_stat_user_tables
WHERE relname IN ('agents', 'server_info', 'server_info_adv', 'server_alerts')
ORDER BY relname;
```

Regular VACUUM normally does not shrink the files on disk. If disk space must
be returned to the operating system, schedule a maintenance window and use one
of these approaches:

- Run `VACUUM (FULL, ANALYZE)` one table at a time while the Hub is stopped.
  It takes an exclusive table lock and requires additional temporary disk space.
- Use `pg_repack` when its prerequisites are satisfied and online rewriting is
  preferred. Follow the version-specific `pg_repack` documentation.

After maintenance, start the Hub and verify that `n_dead_tup`, PostgreSQL CPU,
and table sizes remain stable across at least two 5-minute alert cycles and one
10-minute Agent information cycle.
