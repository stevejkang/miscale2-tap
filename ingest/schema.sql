CREATE TABLE IF NOT EXISTS weight (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  measured_at  TEXT    NOT NULL,
  bucket       TEXT    NOT NULL,
  weight_kg    REAL    NOT NULL,
  impedance    INTEGER,
  source       TEXT    NOT NULL DEFAULT 'esp32',
  UNIQUE (bucket, weight_kg)
);

CREATE INDEX IF NOT EXISTS idx_weight_measured_at ON weight(measured_at);
