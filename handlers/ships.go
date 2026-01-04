package handlers

import (
	"log"
	"net/http"
	"strconv"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// Ship Struct
type Ship struct {
	ID        int
	Name      string
	Code      string
	CreatedAt string
}

// SettingsShipsPage renders the ship management page
func SettingsShipsPage(c *gin.Context) {
	rows, err := db.DB.Query("SELECT id, name, code, created_at FROM fms_ships ORDER BY name ASC")
	var ships []Ship
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s Ship
			// created_at scan might fail if null or format mismatch, handle carefully
			var createdAt []uint8 // raw bytes
			if err := rows.Scan(&s.ID, &s.Name, &s.Code, &createdAt); err == nil {
				s.CreatedAt = string(createdAt)
				ships = append(ships, s)
			}
		}
	}

	c.HTML(http.StatusOK, "settings_ships.html", gin.H{
		"Ships":         ships,
		"ActiveSidebar": "ships",
		"ActiveTab":     "settings",
		"Logo":          GetCompanyLogo(),
	})
}

// CreateShip adds a new ship
func CreateShip(c *gin.Context) {
	name := c.PostForm("name")
	code := c.PostForm("code")

	if name == "" {
		c.String(http.StatusBadRequest, "Name is required")
		return
	}

	_, err := db.DB.Exec("INSERT INTO fms_ships (name, code) VALUES ($1, $2)", name, code)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/ships?success=Kapal+berhasil+ditambahkan!+🚢")
}

// SettingsShipConfigPage renders the sensor configuration for a specific ship
func SettingsShipConfigPage(c *gin.Context) {
	idStr := c.Param("id")
	shipID, _ := strconv.Atoi(idStr)

	// Get Ship Info
	var ship Ship
	err := db.DB.QueryRow("SELECT id, name, code FROM fms_ships WHERE id = $1", shipID).Scan(&ship.ID, &ship.Name, &ship.Code)
	if err != nil {
		c.String(http.StatusNotFound, "Ship not found")
		return
	}

	// Get All Global Sensors
	// We want to list ALL sensors so user can toggle them for this ship
	// But mostly we care about 'Active' global sensors being 'Active' or 'Inactive' for this ship.
	// Actually user might want to enable a globally inactive sensor? Unlikely.
	// Let's assume we list Global Active sensors and allow disabling them for this ship. Or list ALL.
	// User said "dihilangkan salah satu sensornya".

	type ShipSensorConfig struct {
		Code         string
		Name         string
		GlobalActive bool
		ShipActive   bool // effective status
		IsOverride   bool // if entry exists in fms_ship_sensors
	}

	var sensors []ShipSensorConfig

	// Query to join global config with ship specific config
	// Logic: If active in global, standard is True. If entry in ship_sensors exists, use that.
	query := `
		SELECT 
			g.code, g.name, g.is_active as global_status,
			COALESCE(s.is_active, g.is_active) as ship_status,
			CASE WHEN s.ship_id IS NOT NULL THEN true ELSE false END as is_override
		FROM fms_sensor_config g
		LEFT JOIN fms_ship_sensors s ON g.code = s.sensor_code AND s.ship_id = $1
		WHERE g.is_active = true -- Only show globally active sensors to configure? Or all? Let's show Active only to minimize clutter.
		ORDER BY g.display_order ASC
	`

	rows, err := db.DB.Query(query, shipID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s ShipSensorConfig
			rows.Scan(&s.Code, &s.Name, &s.GlobalActive, &s.ShipActive, &s.IsOverride)
			sensors = append(sensors, s)
		}
	}

	c.HTML(http.StatusOK, "settings_ship_config.html", gin.H{
		"Ship":          ship,
		"Sensors":       sensors,
		"ActiveSidebar": "ships",
		"ActiveTab":     "settings",
	})
}

// ToggleShipSensor updates the sensor status for a ship
func ToggleShipSensor(c *gin.Context) {
	shipID, _ := strconv.Atoi(c.Param("id"))
	sensorCode := c.PostForm("sensor_code")
	// toggle action

	// We need to know current state to toggle.
	// Or simpler: User sends "desired state" or just "toggle".
	// Let's implement Toggle.

	// Check if exists
	var currentStatus bool
	err := db.DB.QueryRow("SELECT is_active FROM fms_ship_sensors WHERE ship_id = $1 AND sensor_code = $2", shipID, sensorCode).Scan(&currentStatus)

	if err != nil {
		// Not exists, creating override.
		// If not exists, it followed Global. We need to know Global status to flip it.
		var globalStatus bool
		db.DB.QueryRow("SELECT is_active FROM fms_sensor_config WHERE code = $1", sensorCode).Scan(&globalStatus)

		// New status = NOT Global
		newStatus := !globalStatus
		_, err = db.DB.Exec("INSERT INTO fms_ship_sensors (ship_id, sensor_code, is_active) VALUES ($1, $2, $3)", shipID, sensorCode, newStatus)

	} else {
		// Exists, flip it
		_, err = db.DB.Exec("UPDATE fms_ship_sensors SET is_active = NOT is_active WHERE ship_id = $1 AND sensor_code = $2", shipID, sensorCode)
	}

	if err != nil {
		log.Println(err)
	}

	c.Redirect(http.StatusSeeOther, "/settings/ships/"+strconv.Itoa(shipID)+"?success=Konfigurasi+sensor+diupdate!+📡")
}

// FormSensors returns the HTML fragment for sensor inputs based on ship selection
func FormSensors(c *gin.Context) {
	shipIDStr := c.Query("ship_id")

	var sensors []SensorConfig

	// Default: Global active sensors
	query := `SELECT code, name FROM fms_sensor_config WHERE is_active = true ORDER BY display_order ASC`

	if shipIDStr != "" {
		shipID, _ := strconv.Atoi(shipIDStr)
		// If ship selected, get effective sensors
		query = `
            SELECT DISTINCT g.code, g.name, g.display_order
            FROM fms_sensor_config g
            LEFT JOIN fms_ship_sensors s ON g.code = s.sensor_code AND s.ship_id = $1
            WHERE 
                (g.is_active = true AND (s.is_active IS NULL OR s.is_active = true)) 
                OR (s.is_active = true)
            ORDER BY g.display_order ASC
        `
		rows, err := db.DB.Query(query, shipID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var s SensorConfig
				var displayOrder int // Dummy var for scan
				rows.Scan(&s.Code, &s.Name, &displayOrder)
				sensors = append(sensors, s)
			}
		}
	} else {
		// Fallback global
		rows, err := db.DB.Query(query)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var s SensorConfig
				rows.Scan(&s.Code, &s.Name)
				sensors = append(sensors, s)
			}
		}
	}

	// Render ONLY the options or the whole select?
	// The previous form loop rendered a div per sensor.
	// We should return the whole block of fields.
	c.HTML(http.StatusOK, "partial_sensor_inputs.html", gin.H{
		"Sensors": sensors,
	})
}
