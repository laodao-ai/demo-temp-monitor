-- Keep in sync with db/migrations: every table created there must be dropped here.
DROP TABLE IF EXISTS readings, devices CASCADE;
