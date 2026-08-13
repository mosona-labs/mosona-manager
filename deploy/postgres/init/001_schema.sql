-- ----------------------------
-- Sequence structure for  _id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS " _id_seq";
CREATE SEQUENCE " _id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE " _id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for auth_identity_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "auth_identity_id_seq";
CREATE SEQUENCE "auth_identity_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "auth_identity_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for auth_provider_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "auth_provider_id_seq";
CREATE SEQUENCE "auth_provider_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 2147483647
START 1
CACHE 1;
ALTER SEQUENCE "auth_provider_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for categories_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "categories_id_seq";
CREATE SEQUENCE "categories_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "categories_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for keys_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "keys_id_seq";
CREATE SEQUENCE "keys_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "keys_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for server_alerts_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "server_alerts_id_seq";
CREATE SEQUENCE "server_alerts_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "server_alerts_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for servers_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "servers_id_seq";
CREATE SEQUENCE "servers_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "servers_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for teams_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "teams_id_seq";
CREATE SEQUENCE "teams_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "teams_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for teams_notifications_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "teams_notifications_id_seq";
CREATE SEQUENCE "teams_notifications_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "teams_notifications_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Sequence structure for users_id_seq
-- ----------------------------
DROP SEQUENCE IF EXISTS "users_id_seq";
CREATE SEQUENCE "users_id_seq"
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1;
ALTER SEQUENCE "users_id_seq" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for agents
-- ----------------------------
DROP TABLE IF EXISTS "agents";
CREATE TABLE "agents" (
  "server_id" int8 NOT NULL,
  "agent_uid" char(36) COLLATE "pg_catalog"."default" NOT NULL,
  "status" int2 NOT NULL DEFAULT 0,
  "last_seen_at" timestamp(6) DEFAULT now(),
  "last_ip" varchar(64) COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::character varying,
  "last_version" varchar(64) COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::character varying,
  "public_key" text COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::text,
  "private_key" text COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::text,
  "host" varchar(255) COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::character varying,
  "port" int4 NOT NULL DEFAULT 0
)
;
ALTER TABLE "agents" OWNER TO CURRENT_USER;
COMMENT ON COLUMN "agents"."status" IS '0 - not installed
1 - installed';

-- ----------------------------
-- Table structure for auth_identity
-- ----------------------------
DROP TABLE IF EXISTS "auth_identity";
CREATE TABLE "auth_identity" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "user_id" int8 NOT NULL,
  "provider_id" int4 NOT NULL,
  "subject" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "email" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "quarantined" bool NOT NULL DEFAULT false,
  CONSTRAINT "auth_identity_subject_check" CHECK ("quarantined" OR (("subject"::text <> ''::text) AND ("subject"::text <> '0'::text) AND ("subject"::text !~ '^[[:space:]]|[[:space:]]$'::text)))
)
;
ALTER TABLE "auth_identity" OWNER TO CURRENT_USER;

CREATE TABLE "auth_identity_quarantine_audit" (
  "identity_id" int8 NOT NULL,
  "provider_id" int4 NOT NULL,
  "user_id" int8 NOT NULL,
  "subject" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "reason" text COLLATE "pg_catalog"."default" NOT NULL,
  "quarantined_at" timestamptz(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "auth_identity_quarantine_audit" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for auth_provider
-- ----------------------------
DROP TABLE IF EXISTS "auth_provider";
CREATE TABLE "auth_provider" (
  "id" int4 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 2147483647
START 1
CACHE 1
),
  "name" varchar(64) COLLATE "pg_catalog"."default" NOT NULL,
  "icon" varchar(64) COLLATE "pg_catalog"."default" NOT NULL,
  "protocol" varchar(16) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'oauth2'::character varying,
  "issuer_url" varchar(512) COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::character varying,
  "auth_url" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "token_url" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "userinfo_url" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "scopes" text COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'read:user read:email'::text,
  "subject_field" varchar(255) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'id'::character varying,
  "identity_namespace_version" int8 NOT NULL DEFAULT 1,
  "config_revision" int8 NOT NULL DEFAULT 1,
  "client_id" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "client_secret" text COLLATE "pg_catalog"."default" NOT NULL,
  "skip_2fa" bool NOT NULL,
  "is_enabled" bool NOT NULL,
  "sort" int4 NOT NULL DEFAULT 0,
  "created_at" timestamp(6) NOT NULL DEFAULT now(),
  "updated_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "auth_provider" OWNER TO CURRENT_USER;
COMMENT ON COLUMN "auth_provider"."name" IS 'Google, Github, Company SSO';
COMMENT ON COLUMN "auth_provider"."icon" IS 'Icon name or url';
ALTER TABLE "auth_provider" ADD CONSTRAINT "auth_provider_protocol_check" CHECK ("protocol"::text = ANY (ARRAY['oauth2'::character varying, 'oidc'::character varying]::text[]));
ALTER TABLE "auth_provider" ADD CONSTRAINT "auth_provider_identity_namespace_version_check" CHECK ("identity_namespace_version" > 0);
ALTER TABLE "auth_provider" ADD CONSTRAINT "auth_provider_config_revision_check" CHECK ("config_revision" > 0);

-- ----------------------------
-- Table structure for categories
-- ----------------------------
DROP TABLE IF EXISTS "categories";
CREATE TABLE "categories" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "team" int8 NOT NULL,
  "name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "sort" int4 NOT NULL DEFAULT 0
)
;
ALTER TABLE "categories" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for config
-- ----------------------------
DROP TABLE IF EXISTS "config";
CREATE TABLE "config" (
  "key" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "value" varchar(255) COLLATE "pg_catalog"."default" NOT NULL
)
;
ALTER TABLE "config" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for enroll_tokens
-- ----------------------------
DROP TABLE IF EXISTS "enroll_tokens";
CREATE TABLE "enroll_tokens" (
  "server_id" int8 NOT NULL,
  "token_hash" char(64) COLLATE "pg_catalog"."default" NOT NULL,
  "is_revoked" bool NOT NULL DEFAULT false,
  "created_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "enroll_tokens" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for keys
-- ----------------------------
DROP TABLE IF EXISTS "keys";
CREATE TABLE "keys" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "team_id" int8 NOT NULL,
  "name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "content" bytea NOT NULL,
  "password" bytea,
  "updated_at" timestamp(6) NOT NULL DEFAULT now(),
  "created_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "keys" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for m_team_user
-- ----------------------------
DROP TABLE IF EXISTS "m_team_user";
CREATE TABLE "m_team_user" (
  "team_id" int8 NOT NULL,
  "user_id" int8 NOT NULL,
  "role" int2 NOT NULL DEFAULT 0
)
;
ALTER TABLE "m_team_user" OWNER TO CURRENT_USER;
COMMENT ON COLUMN "m_team_user"."role" IS '0 - Administrator 1 - Read & Terminal Access 2 - Read Only';

-- ----------------------------
-- Table structure for server_alerts
-- ----------------------------
DROP TABLE IF EXISTS "server_alerts";
CREATE TABLE "server_alerts" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "server_id" int8 NOT NULL,
  "item" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "threshold" int4 NOT NULL,
  "for_duration" int4 NOT NULL,
  "last_status" bool,
  "last_notify_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "server_alerts" OWNER TO CURRENT_USER;
COMMENT ON COLUMN "server_alerts"."for_duration" IS 'S';
ALTER TABLE "server_alerts" ADD CONSTRAINT "server_alerts_config_bounds" CHECK (
  ("item" = 'status' AND "threshold" = 0 AND "for_duration" BETWEEN 1 AND 1440)
  OR ("item" IN ('cpu_usage', 'memory_usage', 'disk_usage') AND "threshold" BETWEEN 1 AND 100 AND "for_duration" BETWEEN 1 AND 1440)
  OR ("item" IN ('read_iops', 'write_iops', 'bandwidth') AND "threshold" BETWEEN 1 AND 1000000 AND "for_duration" BETWEEN 1 AND 1440)
  OR ("item" = 'expiry_reminder' AND "threshold" BETWEEN 1 AND 7 AND "for_duration" = 0)
  OR "item" NOT IN ('status', 'cpu_usage', 'memory_usage', 'disk_usage', 'read_iops', 'write_iops', 'bandwidth', 'expiry_reminder')
);

-- ----------------------------
-- Table structure for server_info
-- ----------------------------
DROP TABLE IF EXISTS "server_info";
CREATE TABLE "server_info" (
  "sid" int8 NOT NULL,
  "os" varchar(255) COLLATE "pg_catalog"."default",
  "county" varchar(255) COLLATE "pg_catalog"."default",
  "area" varchar(255) COLLATE "pg_catalog"."default",
  "open_time" timestamptz(6),
  "note" varchar(255) COLLATE "pg_catalog"."default" DEFAULT ''::character varying,
  "provider" varchar(255) COLLATE "pg_catalog"."default",
  "cycle" int2,
  "start_time" timestamptz(6),
  "end_time" timestamptz(6),
  "amount" varchar(255) COLLATE "pg_catalog"."default",
  "auto_renew" bool,
  "bandwidth" varchar(255) COLLATE "pg_catalog"."default",
  "traffic" varchar(255) COLLATE "pg_catalog"."default",
  "traffic_type" int2,
  "note_public" text COLLATE "pg_catalog"."default",
  "online" bool NOT NULL DEFAULT false
)
;
ALTER TABLE "server_info" OWNER TO CURRENT_USER;
COMMENT ON COLUMN "server_info"."cycle" IS '-1 - None 0 - OneTime 1 - Monthly 2 - Quarterly 3 - Semi Annually 4 - Annually';
COMMENT ON COLUMN "server_info"."traffic_type" IS '-1 - None 0 - InBound 1 - OutBound 2 - Both';

-- ----------------------------
-- Table structure for server_info_adv
-- ----------------------------
DROP TABLE IF EXISTS "server_info_adv";
CREATE TABLE "server_info_adv" (
  "sid" int8 NOT NULL,
  "hostname" varchar(255) COLLATE "pg_catalog"."default",
  "cpu_name" varchar(255) COLLATE "pg_catalog"."default",
  "core_c" int2,
  "core_t" int2,
  "kernel" varchar(255) COLLATE "pg_catalog"."default",
  "ip" varchar(255) COLLATE "pg_catalog"."default",
  "arch" varchar(255) COLLATE "pg_catalog"."default"
)
;
ALTER TABLE "server_info_adv" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for servers
-- ----------------------------
DROP TABLE IF EXISTS "servers";
CREATE TABLE "servers" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "team_id" int8 NOT NULL,
  "name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "allow_monitor" bool NOT NULL,
  "allow_terminal" bool NOT NULL,
  "weight" int4 NOT NULL DEFAULT 0,
  "category" int8 NOT NULL,
  "type" int2 NOT NULL,
  "updated_at" timestamp(6) NOT NULL DEFAULT now(),
  "created_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "servers" OWNER TO CURRENT_USER;
COMMENT ON COLUMN "servers"."type" IS '0 - ssh
1 - agent (a)
2 - agent (p)';

-- ----------------------------
-- Table structure for ssh
-- ----------------------------
DROP TABLE IF EXISTS "ssh";
CREATE TABLE "ssh" (
  "server_id" int8 NOT NULL,
  "address" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "port" int4 NOT NULL,
  "username" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "key_id" int8 NOT NULL,
  "password" bytea NOT NULL,
  "host_key" text COLLATE "pg_catalog"."default",
  "trust_legacy_host_key" bool NOT NULL DEFAULT false
)
;
ALTER TABLE "ssh" OWNER TO CURRENT_USER;
ALTER TABLE "ssh" ADD CONSTRAINT "ssh_host_key_not_blank" CHECK ("host_key" IS NULL OR btrim("host_key") <> '');
ALTER TABLE "ssh" ADD CONSTRAINT "ssh_host_key_trust_state" CHECK (("host_key" IS NULL) = "trust_legacy_host_key");

-- ----------------------------
-- Table structure for team_alerts
-- ----------------------------
DROP TABLE IF EXISTS "team_alerts";
CREATE TABLE "team_alerts" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "team_id" int8 NOT NULL,
  "item" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "threshold" int4 NOT NULL,
  "for_duration" int4 NOT NULL
)
;
ALTER TABLE "team_alerts" OWNER TO CURRENT_USER;
ALTER TABLE "team_alerts" ADD CONSTRAINT "team_alerts_config_bounds" CHECK (
  ("item" = 'status' AND "threshold" = 0 AND "for_duration" BETWEEN 1 AND 1440)
  OR ("item" IN ('cpu_usage', 'memory_usage', 'disk_usage') AND "threshold" BETWEEN 1 AND 100 AND "for_duration" BETWEEN 1 AND 1440)
  OR ("item" IN ('read_iops', 'write_iops', 'bandwidth') AND "threshold" BETWEEN 1 AND 1000000 AND "for_duration" BETWEEN 1 AND 1440)
  OR ("item" = 'expiry_reminder' AND "threshold" BETWEEN 1 AND 7 AND "for_duration" = 0)
  OR "item" NOT IN ('status', 'cpu_usage', 'memory_usage', 'disk_usage', 'read_iops', 'write_iops', 'bandwidth', 'expiry_reminder')
);

-- ----------------------------
-- Table structure for team_public_pages
-- ----------------------------
DROP TABLE IF EXISTS "team_public_pages";
CREATE TABLE "team_public_pages" (
  "team_id" int8 NOT NULL,
  "enabled" bool NOT NULL DEFAULT false,
  "name" varchar(64) COLLATE "pg_catalog"."default",
  "domain" varchar(255) COLLATE "pg_catalog"."default",
  "title" varchar(255) COLLATE "pg_catalog"."default",
  "description" text COLLATE "pg_catalog"."default",
  "custom_css" text COLLATE "pg_catalog"."default",
  "created_at" timestamp(6) NOT NULL DEFAULT now(),
  "updated_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "team_public_pages" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for teams
-- ----------------------------
DROP TABLE IF EXISTS "teams";
CREATE TABLE "teams" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "description" varchar(255) COLLATE "pg_catalog"."default" NOT NULL DEFAULT ''::character varying,
  "color" char(7) COLLATE "pg_catalog"."default" NOT NULL,
  "image" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "owner_id" int8 NOT NULL,
  "updated_at" timestamp(6) NOT NULL DEFAULT now(),
  "created_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "teams" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for teams_notifications
-- ----------------------------
DROP TABLE IF EXISTS "teams_notifications";
CREATE TABLE "teams_notifications" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "team_id" int8 NOT NULL,
  "module" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "target" varchar(2048) COLLATE "pg_catalog"."default" NOT NULL
)
;
ALTER TABLE "teams_notifications" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for users
-- ----------------------------
DROP TABLE IF EXISTS "users";
CREATE TABLE "users" (
  "id" int8 NOT NULL GENERATED ALWAYS AS IDENTITY (
INCREMENT 1
MINVALUE  1
MAXVALUE 9223372036854775807
START 1
CACHE 1
),
  "username" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "email" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "password" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "salt" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
  "verified" bool NOT NULL DEFAULT false,
  "is_admin" bool NOT NULL DEFAULT false,
  "totp" varchar(255) COLLATE "pg_catalog"."default",
  "updated_at" timestamp(6) NOT NULL DEFAULT now(),
  "created_at" timestamp(6) NOT NULL DEFAULT now(),
  "login_at" timestamp(6) NOT NULL DEFAULT now(),
  "pwd_at" timestamp(6) NOT NULL DEFAULT now()
)
;
ALTER TABLE "users" OWNER TO CURRENT_USER;

-- ----------------------------
-- Table structure for users_config
-- ----------------------------
DROP TABLE IF EXISTS "users_config";
CREATE TABLE "users_config" (
  "uid" int8 NOT NULL,
  "active_team" int8 NOT NULL
)
;
ALTER TABLE "users_config" OWNER TO CURRENT_USER;

-- ----------------------------
-- Indexes structure for table agents
-- ----------------------------
CREATE INDEX "IDX_AAU" ON "agents" USING btree (
  "agent_uid" COLLATE "pg_catalog"."default" "pg_catalog"."bpchar_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table agents
-- ----------------------------
ALTER TABLE "agents" ADD CONSTRAINT "agents_pkey" PRIMARY KEY ("server_id");

-- ----------------------------
-- Uniques structure for table auth_identity
-- ----------------------------
ALTER TABLE "auth_identity" ADD CONSTRAINT "U_OAUTH" UNIQUE ("provider_id", "subject");

-- ----------------------------
-- Indexes structure for table auth_identity
-- ----------------------------
CREATE INDEX "IDX_AIUP" ON "auth_identity" USING btree (
  "user_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "provider_id" "pg_catalog"."int4_ops" ASC NULLS LAST
);
CREATE UNIQUE INDEX "auth_identity_active_user_provider_unique" ON "auth_identity" USING btree (
  "user_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "provider_id" "pg_catalog"."int4_ops" ASC NULLS LAST
) WHERE "quarantined" = false;

-- ----------------------------
-- Primary Key structure for table auth_identity
-- ----------------------------
ALTER TABLE "auth_identity" ADD CONSTRAINT "auth_identity_pkey" PRIMARY KEY ("id");

ALTER TABLE "auth_identity_quarantine_audit" ADD CONSTRAINT "auth_identity_quarantine_audit_pkey" PRIMARY KEY ("identity_id");

-- ----------------------------
-- Primary Key structure for table auth_provider
-- ----------------------------
ALTER TABLE "auth_provider" ADD CONSTRAINT "auth_provider_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Primary Key structure for table categories
-- ----------------------------
ALTER TABLE "categories" ADD CONSTRAINT "categories_pkey" PRIMARY KEY ("id");
ALTER TABLE "categories" ADD CONSTRAINT "categories_team_id_key" UNIQUE ("team", "id");

-- ----------------------------
-- Indexes structure for table categories
-- ----------------------------
CREATE INDEX "IDX_CTS" ON "categories" USING btree (
  "team" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "sort" "pg_catalog"."int4_ops" ASC NULLS LAST,
  "id" "pg_catalog"."int8_ops" ASC NULLS LAST
);
CREATE UNIQUE INDEX "IDX_CTN" ON "categories" USING btree (
  "team" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "name" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table config
-- ----------------------------
ALTER TABLE "config" ADD CONSTRAINT "config_pkey" PRIMARY KEY ("key");

-- ----------------------------
-- Primary Key structure for table enroll_tokens
-- ----------------------------
ALTER TABLE "enroll_tokens" ADD CONSTRAINT "enroll_tokens_pkey" PRIMARY KEY ("server_id");

-- ----------------------------
-- Indexes structure for table enroll_tokens
-- ----------------------------
CREATE INDEX "IDX_ETTH" ON "enroll_tokens" USING btree (
  "token_hash" COLLATE "pg_catalog"."default" "pg_catalog"."bpchar_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table keys
-- ----------------------------
ALTER TABLE "keys" ADD CONSTRAINT "keys_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table keys
-- ----------------------------
CREATE INDEX "IDX_KTI" ON "keys" USING btree (
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "id" "pg_catalog"."int8_ops" DESC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table m_team_user
-- ----------------------------
ALTER TABLE "m_team_user" ADD CONSTRAINT "m_team_user_pkey" PRIMARY KEY ("team_id", "user_id");

-- ----------------------------
-- Indexes structure for table m_team_user
-- ----------------------------
CREATE INDEX "IDX_MTUU" ON "m_team_user" USING btree (
  "user_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Indexes structure for table server_alerts
-- ----------------------------
CREATE INDEX "IDX_SAS" ON "server_alerts" USING btree (
  "server_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);
CREATE UNIQUE INDEX "IDX_SASI" ON "server_alerts" USING btree (
  "server_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "item" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table server_alerts
-- ----------------------------
ALTER TABLE "server_alerts" ADD CONSTRAINT "server_alerts_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Primary Key structure for table server_info
-- ----------------------------
ALTER TABLE "server_info" ADD CONSTRAINT "server_info_pkey" PRIMARY KEY ("sid");

-- ----------------------------
-- Indexes structure for table server_info
-- ----------------------------
CREATE INDEX "IDX_SIARD" ON "server_info" USING btree (
  "end_time" "pg_catalog"."timestamptz_ops" ASC NULLS LAST
) WHERE "auto_renew" = true AND "cycle" > 0 AND "end_time" IS NOT NULL;

-- ----------------------------
-- Primary Key structure for table server_info_adv
-- ----------------------------
ALTER TABLE "server_info_adv" ADD CONSTRAINT "server_info_adv_pkey" PRIMARY KEY ("sid");

-- ----------------------------
-- Primary Key structure for table servers
-- ----------------------------
ALTER TABLE "servers" ADD CONSTRAINT "servers_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table servers
-- ----------------------------
CREATE INDEX "IDX_STMO" ON "servers" USING btree (
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "allow_monitor" "pg_catalog"."bool_ops" ASC NULLS LAST,
  "weight" "pg_catalog"."int4_ops" DESC NULLS LAST,
  "id" "pg_catalog"."int8_ops" DESC NULLS LAST
);
CREATE INDEX "IDX_STTO" ON "servers" USING btree (
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "allow_terminal" "pg_catalog"."bool_ops" ASC NULLS LAST,
  "weight" "pg_catalog"."int4_ops" DESC NULLS LAST,
  "id" "pg_catalog"."int8_ops" DESC NULLS LAST
);
CREATE INDEX "IDX_SC" ON "servers" USING btree (
  "category" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table ssh
-- ----------------------------
ALTER TABLE "ssh" ADD CONSTRAINT "ssh_pkey" PRIMARY KEY ("server_id");

-- ----------------------------
-- Indexes structure for table team_alerts
-- ----------------------------
CREATE INDEX "IDX_TAT" ON "team_alerts" USING btree (
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);
CREATE UNIQUE INDEX "IDX_TATI" ON "team_alerts" USING btree (
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST,
  "item" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table team_alerts
-- ----------------------------
ALTER TABLE "team_alerts" ADD CONSTRAINT " _pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table team_public_pages
-- ----------------------------
CREATE UNIQUE INDEX "team_public_pages_name_unique" ON "team_public_pages" USING btree (
  LOWER(("name")::text) COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
) WHERE "name" IS NOT NULL;
CREATE UNIQUE INDEX "team_public_pages_domain_unique" ON "team_public_pages" USING btree (
  LOWER(("domain")::text) COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST
) WHERE "domain" IS NOT NULL;

-- ----------------------------
-- Primary Key structure for table team_public_pages
-- ----------------------------
ALTER TABLE "team_public_pages" ADD CONSTRAINT "team_public_pages_pkey" PRIMARY KEY ("team_id");

-- ----------------------------
-- Primary Key structure for table teams
-- ----------------------------
ALTER TABLE "teams" ADD CONSTRAINT "teams_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Indexes structure for table teams
-- ----------------------------
CREATE INDEX "IDX_TOI" ON "teams" USING btree (
  "owner_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Indexes structure for table teams_notifications
-- ----------------------------
CREATE INDEX "IDX_TNTI" ON "teams_notifications" USING btree (
  "team_id" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Primary Key structure for table teams_notifications
-- ----------------------------
ALTER TABLE "teams_notifications" ADD CONSTRAINT "teams_notifications_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Primary Key structure for table users
-- ----------------------------
ALTER TABLE "users" ADD CONSTRAINT "users_pkey" PRIMARY KEY ("id");

-- ----------------------------
-- Uniques structure for table users
-- ----------------------------
ALTER TABLE "users" ADD CONSTRAINT "users_email_unique" UNIQUE ("email");

-- ----------------------------
-- Primary Key structure for table users_config
-- ----------------------------
ALTER TABLE "users_config" ADD CONSTRAINT "users_config_pkey" PRIMARY KEY ("uid");

-- ----------------------------
-- Indexes structure for table users_config
-- ----------------------------
CREATE INDEX "IDX_UCAT" ON "users_config" USING btree (
  "active_team" "pg_catalog"."int8_ops" ASC NULLS LAST
);

-- ----------------------------
-- Foreign Keys structure for table agents
-- ----------------------------
ALTER TABLE "agents" ADD CONSTRAINT "FK_AS" FOREIGN KEY ("server_id") REFERENCES "servers" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table auth_identity
-- ----------------------------
ALTER TABLE "auth_identity" ADD CONSTRAINT "FK_AIP" FOREIGN KEY ("provider_id") REFERENCES "auth_provider" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "auth_identity" ADD CONSTRAINT "FK_AIU" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
-- ----------------------------
-- Foreign Keys structure for table categories
-- ----------------------------
ALTER TABLE "categories" ADD CONSTRAINT "FK_CT" FOREIGN KEY ("team") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table enroll_tokens
-- ----------------------------
ALTER TABLE "enroll_tokens" ADD CONSTRAINT "FK_ETS" FOREIGN KEY ("server_id") REFERENCES "servers" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table keys
-- ----------------------------
ALTER TABLE "keys" ADD CONSTRAINT "FK_KT" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table m_team_user
-- ----------------------------
ALTER TABLE "m_team_user" ADD CONSTRAINT "FK_MTUT" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;
ALTER TABLE "m_team_user" ADD CONSTRAINT "FK_MTUU" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table server_alerts
-- ----------------------------
ALTER TABLE "server_alerts" ADD CONSTRAINT "FK_SAS" FOREIGN KEY ("server_id") REFERENCES "servers" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table server_info
-- ----------------------------
ALTER TABLE "server_info" ADD CONSTRAINT "FK_SIS" FOREIGN KEY ("sid") REFERENCES "servers" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table server_info_adv
-- ----------------------------
ALTER TABLE "server_info_adv" ADD CONSTRAINT "FK_SIDS" FOREIGN KEY ("sid") REFERENCES "servers" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table servers
-- ----------------------------
ALTER TABLE "servers" ADD CONSTRAINT "FK_SC" FOREIGN KEY ("team_id", "category") REFERENCES "categories" ("team", "id") ON DELETE RESTRICT ON UPDATE NO ACTION;
ALTER TABLE "servers" ADD CONSTRAINT "FK_ST" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table ssh
-- ----------------------------
ALTER TABLE "ssh" ADD CONSTRAINT "FK_SS" FOREIGN KEY ("server_id") REFERENCES "servers" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table team_alerts
-- ----------------------------
ALTER TABLE "team_alerts" ADD CONSTRAINT "FK_TST" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table team_public_pages
-- ----------------------------
ALTER TABLE "team_public_pages" ADD CONSTRAINT "FK_TPPT" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table teams
-- ----------------------------
ALTER TABLE "teams" ADD CONSTRAINT "FK_TU" FOREIGN KEY ("owner_id") REFERENCES "users" ("id") ON DELETE RESTRICT ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table teams_notifications
-- ----------------------------
ALTER TABLE "teams_notifications" ADD CONSTRAINT "FK_TNT" FOREIGN KEY ("team_id") REFERENCES "teams" ("id") ON DELETE CASCADE ON UPDATE NO ACTION;

-- ----------------------------
-- Foreign Keys structure for table users_config
-- ----------------------------
ALTER TABLE "users_config" ADD CONSTRAINT "FK_UCT" FOREIGN KEY ("active_team", "uid") REFERENCES "m_team_user" ("team_id", "user_id") ON DELETE CASCADE ON UPDATE NO ACTION;
