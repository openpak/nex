package database

// DatastoreSchema is the DataStore tables (ghost metadata and ratings) of the mk7 schema.
const DatastoreSchema = `
CREATE SCHEMA IF NOT EXISTS mk7;

CREATE TABLE IF NOT EXISTS mk7.datastore_objects (
	data_id bigserial PRIMARY KEY,
	owner_pid numeric(20) NOT NULL,
	size bigint NOT NULL DEFAULT 0,
	name text NOT NULL DEFAULT '',
	data_type int NOT NULL DEFAULT 0,
	meta_binary bytea NOT NULL DEFAULT '',
	permission int NOT NULL DEFAULT 0,
	permission_recipients numeric(20)[] NOT NULL DEFAULT '{}',
	del_permission int NOT NULL DEFAULT 0,
	del_permission_recipients numeric(20)[] NOT NULL DEFAULT '{}',
	created timestamptz NOT NULL DEFAULT now(),
	updated timestamptz NOT NULL DEFAULT now(),
	period int NOT NULL DEFAULT 0,
	status int NOT NULL DEFAULT 0,
	referred_count bigint NOT NULL DEFAULT 0,
	refer_data_id bigint NOT NULL DEFAULT 0,
	flag int NOT NULL DEFAULT 0,
	tags text[] NOT NULL DEFAULT '{}',
	persistence_slot int,
	uploaded boolean NOT NULL DEFAULT false,
	deleted boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS mk7.datastore_ratings (
	data_id bigint NOT NULL REFERENCES mk7.datastore_objects(data_id) ON DELETE CASCADE,
	slot smallint NOT NULL,
	total bigint NOT NULL DEFAULT 0,
	count bigint NOT NULL DEFAULT 0,
	initial bigint NOT NULL DEFAULT 0,
	PRIMARY KEY (data_id, slot)
);
`
