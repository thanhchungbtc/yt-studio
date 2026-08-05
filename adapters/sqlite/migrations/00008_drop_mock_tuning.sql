-- +goose Up

-- The mock providers no longer simulate work: they return as soon as they have
-- written their bytes. These two rows configured that simulation and now
-- configure nothing.
--
-- Settings.Load requires every key the code declares to be present but does not
-- reject extras, so leaving them would not break a boot -- it would leave a
-- "mock" group on the settings screen holding two controls that move nothing.
DELETE FROM settings WHERE key IN ('mock.latency_ms', 'mock.failure_rate_percent');

-- +goose Down

INSERT INTO settings (key, value, type, grp, description, min_value, max_value, updated_at)
VALUES
    ('mock.latency_ms', '40', 'int', 'mock',
     'Simulated provider work per unit, scaled per task kind.',
     0, 600000, CAST(strftime('%s', 'now') AS INTEGER) * 1000000000),
    ('mock.failure_rate_percent', '0', 'int', 'mock',
     'Injected transient failure rate, to exercise retries.',
     0, 100, CAST(strftime('%s', 'now') AS INTEGER) * 1000000000);
