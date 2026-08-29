# Testing

## Unit and package tests

Several Hub packages validate configuration during package initialization. Set the following harmless placeholder values even when a test uses mocked databases:

```sh
export POSTGRES_PORT=5432
export POSTGRES_USER=test
export POSTGRES_PASS=test
export POSTGRES_DB=test
export INFLUXDB_ORG=test
export INFLUXDB_TOKEN=test
export REDIS_PORT=6379

go test ./...
```

The Active Agent security tests start fake WebSocket servers on temporary loopback ports. The test environment must allow binding to `127.0.0.1`.

Run the connection packages with the race detector after changing connection or lifecycle code:

```sh
go test -race ./agent/active ./internal/connect/active ./internal/connect/conn -count=1
```

## PostgreSQL migration tests

Migration execution tests require a disposable PostgreSQL database. The Active Agent protocol migration test uses:

```sh
export ACTIVE_AGENT_PROTOCOL_TEST_DATABASE_URL='postgres://mm:mm@127.0.0.1:55432/mm_db?sslmode=disable'
go test ./internal/db -run TestActiveAgentProtocolMigrationPostgres -count=1
```

The test creates only temporary tables and executes the migration twice to verify idempotency. Other optional PostgreSQL migration and tenant tests use these variables:

- `SSH_HOST_KEY_TEST_DATABASE_URL`
- `SERVER_CATEGORY_TEST_DATABASE_URL`
- `ALERT_CONFIG_TEST_DATABASE_URL`
- `ALERT_TENANT_TEST_DATABASE_URL`
- `TEAM_OWNER_DELETE_TEST_DATABASE_URL`
- `TEAM_OWNER_ROLE_TEST_DATABASE_URL`

Tests requiring one of these variables skip when it is unset.
