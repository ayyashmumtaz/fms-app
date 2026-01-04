package handlers

import (
	"fms-app/db"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// SensorColumn represents a sensor header in the batch table
type SensorColumn struct {
	Code string
	Name string
}

// ShipBatchRow represents a row for a ship with its sensor configs
type ShipBatchRow struct {
	ID   int
	Name string
	Code string
	// Config maps sensor_code -> is_allowed (based on settings)
	Config map[string]bool
}

// BatchInputPage renders the batch input form as a checkbox matrix
func BatchInputPage(c *gin.Context) {
	// 1. Get All Active Global Sensors (Sorted) is_active=true
	sRows, err := db.DB.Query("SELECT code, name FROM fms_sensor_config WHERE is_active = true ORDER BY display_order ASC")
	if err != nil {
		c.String(http.StatusInternalServerError, "Error fetching sensors: %v", err)
		return
	}
	defer sRows.Close()

	var columns []SensorColumn
	for sRows.Next() {
		var col SensorColumn
		sRows.Scan(&col.Code, &col.Name)
		columns = append(columns, col)
	}

	// 2. Get All Active Ships (Rows)
	rows, err := db.DB.Query("SELECT id, name, code FROM fms_ships ORDER BY name ASC")
	if err != nil {
		c.String(http.StatusInternalServerError, "Error fetching ships: %v", err)
		return
	}
	defer rows.Close()

	var ships []ShipBatchRow
	for rows.Next() {
		var s ShipBatchRow
		s.Config = make(map[string]bool)
		rows.Scan(&s.ID, &s.Name, &s.Code)
		ships = append(ships, s)
	}

	// 3. Get Ship-Specific Overrides (Check fms_ship_sensors)
	overrides := make(map[int]map[string]bool)
	oRows, err := db.DB.Query("SELECT ship_id, sensor_code, is_active FROM fms_ship_sensors")
	if err == nil {
		defer oRows.Close()
		for oRows.Next() {
			var sid int
			var code string
			var active bool
			oRows.Scan(&sid, &code, &active)
			if overrides[sid] == nil {
				overrides[sid] = make(map[string]bool)
			}
			overrides[sid][code] = active
		}
	}

	// 4. Build Config Map for each Ship
	for i := range ships {
		sid := ships[i].ID
		for _, col := range columns {
			allowed := true
			if ovMap, ok := overrides[sid]; ok {
				if active, found := ovMap[col.Code]; found {
					allowed = active
				}
			}
			ships[i].Config[col.Code] = allowed
		}
	}

	// 5. Get Active Projects for Dropdown
	type Project struct {
		Code string
		Name string
	}
	var projects []Project
	pRows, err := db.DB.Query("SELECT code, name FROM fms_projects WHERE is_active = true ORDER BY name ASC")
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var p Project
			pRows.Scan(&p.Code, &p.Name)
			projects = append(projects, p)
		}
	}

	currentTime := time.Now()
	currentPeriod := currentTime.Format("2006-01")

	c.HTML(http.StatusOK, "batch_input.html", gin.H{
		"Ships":         ships,
		"Columns":       columns,
		"Projects":      projects,
		"CurrentPeriod": currentPeriod,
		"ActiveTab":     "batch",
		"Logo":          GetCompanyLogo(),
	})
}

// BatchSubmit handles the submission of batch checkbox matrix
func BatchSubmit(c *gin.Context) {
	reportPeriod := c.PostForm("report_period")
	projectCode := c.PostForm("project_code")

	if reportPeriod == "" {
		c.String(http.StatusBadRequest, "Report period is required")
		return
	}

	periodDate, err := time.Parse("2006-01", reportPeriod)
	reportDateStr := reportPeriod + "-01"
	reportCodeSuffix := ""
	if err == nil {
		reportCodeSuffix = periodDate.Format("Jan 2006")
	}

	// We need Ship CODE for unique report code generation
	rows, err := db.DB.Query("SELECT id, name, code FROM fms_ships")
	if err != nil {
		c.String(http.StatusInternalServerError, "DB Error: %v", err)
		return
	}
	defer rows.Close()

	tx, err := db.DB.Begin()
	if err != nil {
		c.String(http.StatusInternalServerError, "DB Error: %v", err)
		return
	}
	defer tx.Rollback()

	count := 0

	for rows.Next() {
		var sid int
		var sName, sCode string
		rows.Scan(&sid, &sName, &sCode)
		sidStr := strconv.Itoa(sid)

		// Only process selected ships
		if c.PostForm("status_"+sidStr) != "on" {
			continue
		}

		// UNIQUE CODE GENERATION: Project + ShipCode + Period
		fullCode := fmt.Sprintf("%s %s %s", projectCode, sCode, reportCodeSuffix)
		fullCode = strings.TrimSpace(fullCode)

		sensorsStatus := make(map[string]bool)

		// Fetch dynamic sensor list
		sConfigRows, _ := db.DB.Query("SELECT code FROM fms_sensor_config")
		var allCodes []string
		if sConfigRows != nil {
			for sConfigRows.Next() {
				var sc string
				sConfigRows.Scan(&sc)
				allCodes = append(allCodes, sc)
			}
			sConfigRows.Close()
		}

		// Prepare variables for INSERT
		// We will insert 0 for numeric columns.
		// 'gps' column appears to be BOOLEAN based on error message.
		var devCond, gpsStatus bool

		// Map checkboxes to JSON status
		for _, code := range allCodes {
			inputName := fmt.Sprintf("sensor_%d_%s", sid, code)
			val := c.PostForm(inputName)
			isOn := (val == "on")

			// Store real status in JSON map
			sensorsStatus[code] = isOn

			// Specific mapping for standard columns
			if code == "device_condition" {
				devCond = isOn
			}

			if code == "gps" {
				gpsStatus = isOn
			}

			// For RPM and Flowmeters (NUMERIC), we leave them as 0.
		}

		jsonParts := []string{}
		for k, v := range sensorsStatus {
			jsonParts = append(jsonParts, fmt.Sprintf(`"%s": %v`, k, v))
		}
		jsonStr := "{" + strings.Join(jsonParts, ",") + "}"

		// Use 0 for numeric columns to avoid type errors
		_, err := tx.Exec(`
			INSERT INTO fms_device_reports 
			(code, report_date, ship_name, device_condition, gps, rpm_me_port, rpm_me_stbd, flowmeter_input, flowmeter_output, flowmeter_bunker, sensors_data)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, fullCode, reportDateStr, sName, devCond, gpsStatus, 0, 0, 0, 0, 0, jsonStr)

		if err != nil {
			log.Printf("Batch Insert Error %s: %v", sName, err)
			// Return error to user instead of breaking transaction silently
			c.String(http.StatusInternalServerError, "Gagal menyimpan laporan untuk %s. \nError: %v. \nKemungkinan duplikat laporan untuk periode ini.", sName, err)
			return
		}
		count++
	}

	if err := tx.Commit(); err != nil {
		c.String(http.StatusInternalServerError, "Commit Error: %v", err)
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/batch-input?success=Batch+sukses!+%d+laporan+disimpan.✅", count))
}
