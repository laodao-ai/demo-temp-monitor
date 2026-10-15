-- Demo database schema. No incremental migrations: scripts/db-reset.sh rebuilds from zero.
CREATE TABLE devices (
    id            text        PRIMARY KEY CHECK (id ~ '^[a-z0-9-]{1,64}$'),
    registered_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE readings (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id     text             NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    temperature_c double precision NOT NULL,
    recorded_at   timestamptz      NOT NULL DEFAULT now()
);

CREATE INDEX readings_device_id_idx ON readings (device_id, id);
