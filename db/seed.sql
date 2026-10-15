-- Demo data. Seeding does not raise alarms: alarms only fire on the report path.
INSERT INTO devices (id) VALUES ('demo-01'), ('demo-02'), ('demo-03');

INSERT INTO readings (device_id, temperature_c) VALUES
    ('demo-01', 21.5),
    ('demo-01', 22.0),
    ('demo-02', 30.2),
    ('demo-02', 34.8),
    ('demo-03', 45.0),
    ('demo-03', 55.5);
