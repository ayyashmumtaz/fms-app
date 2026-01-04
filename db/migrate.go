package db

import (
	"fmt"
	"os"
)

func Migrate() error {
	q := `
CREATE TABLE IF NOT EXISTS fms_device_reports (
  id SERIAL PRIMARY KEY,
  code VARCHAR(50) NOT NULL,
  report_date DATE NOT NULL,
  ship_name VARCHAR(255) NOT NULL,
  
  -- Legacy columns (kept for backward compatibility during migration)
  device_condition BOOLEAN DEFAULT FALSE,
  gps BOOLEAN DEFAULT FALSE,
  rpm_me_port BOOLEAN DEFAULT FALSE,
  rpm_me_stbd BOOLEAN DEFAULT FALSE,
  flowmeter_input BOOLEAN DEFAULT FALSE,
  flowmeter_output BOOLEAN DEFAULT FALSE,
  flowmeter_bunker BOOLEAN DEFAULT FALSE,

  -- New Flexible Column
  sensors_data JSONB DEFAULT '{}',

  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fms_sensor_config (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    display_order INT DEFAULT 0
);

CREATE TABLE IF NOT EXISTS fms_ships (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    code VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fms_ship_sensors (
    ship_id INT NOT NULL REFERENCES fms_ships(id) ON DELETE CASCADE,
    sensor_code VARCHAR(50) NOT NULL, -- No foreign key restricted to allow flexible config even if config changes slightly
    is_active BOOLEAN DEFAULT TRUE,
    PRIMARY KEY (ship_id, sensor_code)
);

CREATE TABLE IF NOT EXISTS fms_projects (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS fms_app_config (
    key VARCHAR(50) PRIMARY KEY,
    value TEXT
);
INSERT INTO fms_app_config (key, value) VALUES ('company_logo', '/static/images/logo-placeholder.png') ON CONFLICT DO NOTHING;


CREATE INDEX IF NOT EXISTS idx_fms_device_reports_code ON fms_device_reports(code);
CREATE INDEX IF NOT EXISTS idx_fms_device_reports_date ON fms_device_reports(report_date);
CREATE INDEX IF NOT EXISTS idx_fms_device_reports_ship ON fms_device_reports(ship_name);
`
	if _, err := DB.Exec(q); err != nil {
		return err
	}

	// Migrate existing ships to master table
	_, _ = DB.Exec(`
        INSERT INTO fms_ships (name) 
        SELECT DISTINCT ship_name FROM fms_device_reports 
        WHERE ship_name IS NOT NULL AND ship_name != ''
        ON CONFLICT (name) DO NOTHING;
    `)

	// Seed default sensors if table is empty
	_, _ = DB.Exec(`
    INSERT INTO fms_sensor_config (code, name, is_active, display_order)
    VALUES 
        ('device_condition', 'Device Condition', true, 1),
        ('gps', 'GPS', true, 2),
        ('rpm_me_port', 'RPM ME Port', true, 3),
        ('rpm_me_stbd', 'RPM ME Stbd', true, 4),
        ('flowmeter_input', 'Flowmeter Input', true, 5),
        ('flowmeter_output', 'Flowmeter Output', true, 6),
        ('flowmeter_bunker', 'Flowmeter Bunker', true, 7)
    ON CONFLICT (code) DO NOTHING;
    `)

	// Seed Projects
	_, _ = DB.Exec(`
    INSERT INTO fms_projects (code, name) 
    VALUES ('FMS', 'Fuel Monitoring System')
    ON CONFLICT (code) DO NOTHING;
    `)

	// Ensure sensors_data column exists if migrating existing DB
	_, _ = DB.Exec(`ALTER TABLE fms_device_reports ADD COLUMN IF NOT EXISTS sensors_data JSONB DEFAULT '{}';`)

	// Optional seed sample rows
	if os.Getenv("SEED_SAMPLE") == "true" {
		_, _ = DB.Exec(`
INSERT INTO fms_device_reports (code, report_date, ship_name, device_condition, gps, rpm_me_port, rpm_me_stbd, flowmeter_input, flowmeter_output, flowmeter_bunker, sensors_data)
VALUES
('FMS Dec 2025', '2025-12-01', 'TB CELEBES SEJATI 01', true, true, true, true, true, true, true, '{"device_condition": true, "gps": true, "rpm_me_port": true, "rpm_me_stbd": true, "flowmeter_input": true, "flowmeter_output": true, "flowmeter_bunker": true}'),
('FMS Dec 2025', '2025-12-01', 'TB ENTEBE MEGASTAR 63', true, true, false, false, false, true, true, '{"device_condition": true, "gps": true, "rpm_me_port": false, "rpm_me_stbd": false, "flowmeter_input": false, "flowmeter_output": true, "flowmeter_bunker": true}'),
('FMS Dec 2025', '2025-12-01', 'TB ENTEBE MEGASTAR 67', true, true, false, false, false, true, true, '{"device_condition": true, "gps": true, "rpm_me_port": false, "rpm_me_stbd": false, "flowmeter_input": false, "flowmeter_output": true, "flowmeter_bunker": true}')
ON CONFLICT DO NOTHING;
`)
	}

	fmt.Println("migration: ok")
	return nil
}
