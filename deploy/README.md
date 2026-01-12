# Deploy (Docker Compose)

This guide provides instructions on how to deploy Mosona Manager using Docker.

```bash
git clone https://github.com/mosona-labs/mosona-manager.git
cd mosona-manager/deploy/
cp .env.example .env
```
### Edit `.env` File

Open the `.env` file and modify the following variables as needed:

- `APP_PORT`: HTTP port (default: `8080`).
- `PG_PASSWORD`: Change the Postgres password for security.
- `INFLUX_PASSWORD`: Change the InfluxDB password for security.
- `INFLUX_TOKEN`: Set up a long and complex InfluxDB token.

### Start Services

```bash
docker compose up -d
docker compose logs -f bootstrap app
```

### Access the Application

Open your web browser and navigate to `http://<your-server-ip>:<APP_PORT>` (default port is `8080`).

----

### Upgrade

Edit `.env` file, set `RELEASE_TAG` to the desired version or `latest`, then run:

```bash
docker compose up -d --no-deps bootstrap
docker compose up -d app
```