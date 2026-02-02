package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// Dashboard shows the performance dashboard with summary of all codes
func Dashboard(c *gin.Context) {
	// Get all available codes
	rows, err := db.DB.Query(`
		SELECT DISTINCT code 
		FROM fms_device_reports 
		ORDER BY code DESC
	`)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}
	defer rows.Close()

	var codes []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err == nil {
			codes = append(codes, code)
		}
	}

	// Get summary for each code
	var summaries []RekapSummary
	for _, code := range codes {
		var totalShips int
		var totalOnline, totalOffline *int // Use pointers to handle NULL

		err := db.DB.QueryRow(`
			SELECT 
				COUNT(*) as total_ships,
				SUM(
					CASE WHEN device_condition THEN 1 ELSE 0 END +
					CASE WHEN gps THEN 1 ELSE 0 END +
					CASE WHEN rpm_me_port THEN 1 ELSE 0 END +
					CASE WHEN rpm_me_stbd THEN 1 ELSE 0 END +
					CASE WHEN flowmeter_input THEN 1 ELSE 0 END +
					CASE WHEN flowmeter_output THEN 1 ELSE 0 END +
					CASE WHEN flowmeter_bunker THEN 1 ELSE 0 END
				) as total_online,
				SUM(
					CASE WHEN NOT device_condition THEN 1 ELSE 0 END +
					CASE WHEN NOT gps THEN 1 ELSE 0 END +
					CASE WHEN NOT rpm_me_port THEN 1 ELSE 0 END +
					CASE WHEN NOT rpm_me_stbd THEN 1 ELSE 0 END +
					CASE WHEN NOT flowmeter_input THEN 1 ELSE 0 END +
					CASE WHEN NOT flowmeter_output THEN 1 ELSE 0 END +
					CASE WHEN NOT flowmeter_bunker THEN 1 ELSE 0 END
				) as total_offline
			FROM fms_device_reports
			WHERE code = $1
		`, code).Scan(&totalShips, &totalOnline, &totalOffline)

		if err != nil {
			continue
		}

		online := 0
		offline := 0
		if totalOnline != nil {
			online = *totalOnline
		}
		if totalOffline != nil {
			offline = *totalOffline
		}

		totalDevices := online + offline
		var onlinePercent, offlinePercent float64
		if totalDevices > 0 {
			onlinePercent = float64(online) / float64(totalDevices) * 100
			offlinePercent = float64(offline) / float64(totalDevices) * 100
		}

		summaries = append(summaries, RekapSummary{
			Code:              code,
			TotalShips:        totalShips,
			TotalDevices:      totalDevices,
			TotalOnline:       online,
			TotalOffline:      offline,
			OnlinePercentage:  onlinePercent,
			OfflinePercentage: offlinePercent,
		})
	}

	// Get latest 10 reports
	var latestReports []DeviceReport
	lRows, err := db.DB.Query(`
		SELECT id, code, report_date, ship_name, 
		       device_condition, gps, rpm_me_port, rpm_me_stbd,
		       flowmeter_input, flowmeter_output, flowmeter_bunker,
		       sensors_data,
		       created_at, updated_at
		FROM fms_device_reports
		ORDER BY created_at DESC
		LIMIT 10
	`)
	if err == nil {
		defer lRows.Close()
		for lRows.Next() {
			var r DeviceReport
			var sensorsJson []byte
			if err := lRows.Scan(
				&r.ID, &r.Code, &r.ReportDate, &r.ShipName,
				&r.DeviceCondition, &r.GPS, &r.RpmMEPort, &r.RpmMEStbd,
				&r.FlowmeterInput, &r.FlowmeterOutput, &r.FlowmeterBunker,
				&sensorsJson,
				&r.CreatedAt, &r.UpdatedAt,
			); err != nil {
				continue
			}

			if len(sensorsJson) > 0 {
				_ = json.Unmarshal(sensorsJson, &r.SensorsData)
			}
			r.CalculateTotals()
			latestReports = append(latestReports, r)
		}
	}

	// Fetch sensors from DB for table headers
	var sensors []SensorConfig
	sRows, err := db.DB.Query("SELECT code, name FROM fms_sensor_config WHERE is_active = true ORDER BY display_order ASC")
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var s SensorConfig
			if err := sRows.Scan(&s.Code, &s.Name); err == nil {
				sensors = append(sensors, s)
			}
		}
	}

	// Fetch trouble reports (latest report per ship with offline sensors)
	var troubleReports []DeviceReport
	tRows, err := db.DB.Query(`
		SELECT DISTINCT ON (ship_name) id, code, report_date, ship_name, 
		       device_condition, gps, rpm_me_port, rpm_me_stbd,
		       flowmeter_input, flowmeter_output, flowmeter_bunker,
		       sensors_data,
		       created_at, updated_at
		FROM fms_device_reports
		ORDER BY ship_name, created_at DESC
	`)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var r DeviceReport
			var sensorsJson []byte
			if err := tRows.Scan(
				&r.ID, &r.Code, &r.ReportDate, &r.ShipName,
				&r.DeviceCondition, &r.GPS, &r.RpmMEPort, &r.RpmMEStbd,
				&r.FlowmeterInput, &r.FlowmeterOutput, &r.FlowmeterBunker,
				&sensorsJson,
				&r.CreatedAt, &r.UpdatedAt,
			); err != nil {
				continue
			}

			if len(sensorsJson) > 0 {
				_ = json.Unmarshal(sensorsJson, &r.SensorsData)
			}
			r.CalculateTotals()

			// If has offline sensors, add to trouble list
			if r.OfflineTotal > 0 {
				troubleReports = append(troubleReports, r)
			}
		}
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Summaries":      summaries,
		"Codes":          codes,
		"LatestReports":  latestReports,
		"TroubleReports": troubleReports,
		"Sensors":        sensors,
		"CurrentYear":    time.Now().Year(),
		"ActiveTab":      "dashboard",
		"Logo":           GetCompanyLogo(),
	})
}

// GetNotificationCount returns the HTML fragment for the notification badge
func GetNotificationCount(c *gin.Context) {
	// Fetch trouble reports count
	count := 0
	rows, err := db.DB.Query(`
		SELECT DISTINCT ON (ship_name) id, sensors_data
		FROM fms_device_reports
		ORDER BY ship_name, created_at DESC
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id int
			var sensorsJson []byte
			if err := rows.Scan(&id, &sensorsJson); err == nil {
				var sensorsData map[string]bool
				if len(sensorsJson) > 0 {
					_ = json.Unmarshal(sensorsJson, &sensorsData)
				}

				// Check if any sensor is offline
				hasOffline := false
				if len(sensorsData) > 0 {
					for _, status := range sensorsData {
						if !status {
							hasOffline = true
							break
						}
					}
				}

				if hasOffline {
					count++
				}
			}
		}
	}

	if count > 0 {
		c.HTML(http.StatusOK, "notification_badge.html", gin.H{
			"Count": count,
		})
	} else {
		// Return empty div if no notifications
		c.String(http.StatusOK, `<!-- No notifications -->`)
	}
}

// ResolveAlert marks a specific sensor in a report as fixed (Online)
func ResolveAlert(c *gin.Context) {
	idStr := c.Param("id")
	sensorCode := c.Query("sensor")

	if sensorCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Sensor code is required"})
		return
	}

	// 1. Fetch current data
	var sensorsJson []byte
	err := db.DB.QueryRow(`
		SELECT sensors_data 
		FROM fms_device_reports 
		WHERE id = $1
	`, idStr).Scan(&sensorsJson)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Report not found"})
		return
	}

	// 2. Unmarshal and update sensors
	sensorsMap := make(map[string]bool)
	if len(sensorsJson) > 0 {
		_ = json.Unmarshal(sensorsJson, &sensorsMap)
	}

	// Update specific sensor
	sensorsMap[sensorCode] = true

	// Marshal back
	newSensorsJson, _ := json.Marshal(sensorsMap)

	// 3. Update DB (JSON and Legacy Column if applicable)
	// We construct the query dynamically based on the sensor code if it maps to a legacy column
	query := `UPDATE fms_device_reports SET sensors_data = $1, updated_at = CURRENT_TIMESTAMP`
	args := []interface{}{newSensorsJson, idStr}

	// Legacy columns mapping
	legacyColumns := map[string]string{
		"device_condition": "device_condition",
		"gps":              "gps",
		"rpm_me_port":      "rpm_me_port",
		"rpm_me_stbd":      "rpm_me_stbd",
		"flowmeter_input":  "flowmeter_input",
		"flowmeter_output": "flowmeter_output",
		"flowmeter_bunker": "flowmeter_bunker",
	}

	if col, isLegacy := legacyColumns[sensorCode]; isLegacy {
		query = `UPDATE fms_device_reports SET ` + col + ` = true, sensors_data = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	} else {
		query += ` WHERE id = $2`
	}

	_, err = db.DB.Exec(query, args...)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MonthlyReport shows detailed report for a specific code/month
func MonthlyReport(c *gin.Context) {
	code := c.Query("code")
	project := c.Query("project")
	dateStr := c.Query("date")

	// Filter by Project + Date if provided (New Filter Logic)
	// Filter by Project + Date if provided (New Filter Logic)
	if project != "" && dateStr != "" {
		// dateStr is YYYY-MM (e.g., 2025-12)
		parsedDate, err := time.Parse("2006-01", dateStr)
		if err == nil {
			// Convert to "Dec 2025" format
			period := parsedDate.Format("Jan 2006")
			// Use wildcard for matching: PROJECT % PERIOD
			code = project + "%" + period
		}
	}

	if code == "" {
		// Default view: current month
		code = "FMS % " + time.Now().Format("Jan 2006")
	}

	// Get all reports for this code
	rows, err := db.DB.Query(`
		SELECT id, code, report_date, ship_name, 
		       device_condition, gps, rpm_me_port, rpm_me_stbd,
		       flowmeter_input, flowmeter_output, flowmeter_bunker,
		       sensors_data,
		       created_at, updated_at
		FROM fms_device_reports
		WHERE code LIKE $1
		ORDER BY report_date ASC, ship_name ASC
	`, code)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}
	defer rows.Close()

	var reports []DeviceReport
	for rows.Next() {
		var r DeviceReport
		var sensorsJson []byte
		if err := rows.Scan(
			&r.ID, &r.Code, &r.ReportDate, &r.ShipName,
			&r.DeviceCondition, &r.GPS, &r.RpmMEPort, &r.RpmMEStbd,
			&r.FlowmeterInput, &r.FlowmeterOutput, &r.FlowmeterBunker,
			&sensorsJson,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			continue
		}

		// Unmarshal dynamic sensors data
		if len(sensorsJson) > 0 {
			_ = json.Unmarshal(sensorsJson, &r.SensorsData)
		}

		r.CalculateTotals()
		reports = append(reports, r)
	}

	// Get all available codes for navigation
	codeRows, _ := db.DB.Query(`SELECT DISTINCT code FROM fms_device_reports ORDER BY code DESC`)
	var codes []string
	if codeRows != nil {
		defer codeRows.Close()
		for codeRows.Next() {
			var c string
			if codeRows.Scan(&c) == nil {
				codes = append(codes, c)
			}
		}
	}

	// Fetch sensors from DB for table headers
	var sensors []SensorConfig
	sRows, err := db.DB.Query("SELECT code, name FROM fms_sensor_config WHERE is_active = true ORDER BY display_order ASC")
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var s SensorConfig
			if err := sRows.Scan(&s.Code, &s.Name); err == nil {
				sensors = append(sensors, s)
			}
		}
	}

	// Get active projects for filter dropdown
	var projects []string
	pRows, err := db.DB.Query("SELECT code FROM fms_projects WHERE is_active = true ORDER BY code ASC")
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var pCode string
			if err := pRows.Scan(&pCode); err == nil {
				projects = append(projects, pCode)
			}
		}
	}

	c.HTML(http.StatusOK, "monthly_report.html", gin.H{
		"Code":           code,
		"Reports":        reports,
		"Codes":          codes,
		"Sensors":        sensors,
		"Projects":       projects,
		"CurrentProject": project, // Added current project for selection state
		"ActiveTab":      "report",
		"Logo":           GetCompanyLogo(),
	})
}
