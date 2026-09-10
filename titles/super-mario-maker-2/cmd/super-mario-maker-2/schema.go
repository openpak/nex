package main

import "database/sql"

// createTables makes the tables this server owns. The matchmaking, tracking and
// utility schemas are not here: nex-protocols-common-go creates its own when its
// managers are set, and duplicating them would be one more thing to keep in step.
//
// Timestamps are `timestamp` without a time zone deliberately — that is the only
// column type nex-go's DateTime knows how to scan.
func createTables(db *sql.DB) error {
	_, err := db.Exec(`
CREATE SCHEMA IF NOT EXISTS datastore;
CREATE TABLE IF NOT EXISTS datastore.objects (
	data_id bigserial PRIMARY KEY,
	owner_pid numeric(20) NOT NULL,
	size bigint NOT NULL DEFAULT 0,
	name text NOT NULL DEFAULT '',
	data_type int NOT NULL DEFAULT 0,
	meta_binary bytea NOT NULL DEFAULT '',
	permission int NOT NULL DEFAULT 0,
	permission_recipients bigint[] NOT NULL DEFAULT '{}',
	del_permission int NOT NULL DEFAULT 0,
	del_recipients bigint[] NOT NULL DEFAULT '{}',
	created_time timestamp NOT NULL DEFAULT now(),
	updated_time timestamp NOT NULL DEFAULT now(),
	referred_time timestamp,
	expire_time timestamp,
	period int NOT NULL DEFAULT 0,
	status int NOT NULL DEFAULT 0,
	referred_cnt bigint NOT NULL DEFAULT 0,
	refer_data_id bigint NOT NULL DEFAULT 0,
	flag bigint NOT NULL DEFAULT 0,
	tags text[] NOT NULL DEFAULT '{}',
	persistence_slot int,
	access_password numeric(20) NOT NULL DEFAULT 0,
	update_password numeric(20) NOT NULL DEFAULT 0,
	upload_completed boolean NOT NULL DEFAULT false,
	deleted boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS objects_owner ON datastore.objects (owner_pid) WHERE deleted = false;
CREATE INDEX IF NOT EXISTS objects_slot ON datastore.objects (owner_pid, persistence_slot) WHERE persistence_slot IS NOT NULL;

CREATE TABLE IF NOT EXISTS datastore.ratings (
	data_id bigint NOT NULL REFERENCES datastore.objects (data_id),
	slot smallint NOT NULL,
	total_value bigint NOT NULL DEFAULT 0,
	count bigint NOT NULL DEFAULT 0,
	initial_value bigint NOT NULL DEFAULT 0,
	range_min int NOT NULL DEFAULT 0,
	range_max int NOT NULL DEFAULT 0,
	lock_type int NOT NULL DEFAULT 0,
	flag int NOT NULL DEFAULT 0,
	internal_flag int NOT NULL DEFAULT 0,
	period_hour int NOT NULL DEFAULT 0,
	period_duration int NOT NULL DEFAULT 0,
	PRIMARY KEY (data_id, slot)
);

CREATE SCHEMA IF NOT EXISTS ranking;
CREATE TABLE IF NOT EXISTS ranking.scores (
	pid numeric(20) NOT NULL,
	category bigint NOT NULL,
	score bigint NOT NULL,
	order_by int NOT NULL DEFAULT 0,
	groups bytea NOT NULL DEFAULT '',
	param numeric(20) NOT NULL DEFAULT 0,
	unique_id numeric(20) NOT NULL DEFAULT 0,
	updated_time timestamp NOT NULL DEFAULT now(),
	PRIMARY KEY (pid, category)
);
CREATE INDEX IF NOT EXISTS scores_board ON ranking.scores (category, score);

CREATE TABLE IF NOT EXISTS ranking.common_data (
	pid numeric(20) PRIMARY KEY,
	unique_id numeric(20) NOT NULL DEFAULT 0,
	data bytea NOT NULL DEFAULT ''
);
`)
	return err
}
