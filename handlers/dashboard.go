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

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Summaries":     summaries,
		"Codes":         codes,
		"LatestReports": latestReports,
		"Sensors":       sensors,
		"CurrentYear":   time.Now().Year(),
		"ActiveTab":     "dashboard",
		"Logo":          GetCompanyLogo(),
	})
}

// MonthlyReport shows detailed report for a specific code/month
func MonthlyReport(c *gin.Context) {
	code := c.Query("code")
	project := c.Query("project")
	dateStr := c.Query("date")

	// Filter by Project + Date if provided (New Filter Logic)
	if project != "" && dateStr != "" {
		// dateStr is YYYY-MM (e.g., 2025-12)
		parsedDate, err := time.Parse("2006-01", dateStr)
		if err == nil {
			// Convert to "Dec 2025" format
			period := parsedDate.Format("Jan 2006")
			code = project + " " + period
		}
	}

	if code == "" {
		code = time.Now().Format("FMS Jan 2006")
	}

	// Get all reports for this code
	rows, err := db.DB.Query(`
		SELECT id, code, report_date, ship_name, 
		       device_condition, gps, rpm_me_port, rpm_me_stbd,
		       flowmeter_input, flowmeter_output, flowmeter_bunker,
		       sensors_data,
		       created_at, updated_at
		FROM fms_device_reports
		WHERE code = $1
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

	c.HTML(http.StatusOK, "monthly_report.html", gin.H{
		"Code":      code,
		"Reports":   reports,
		"Codes":     codes,
		"Sensors":   sensors, // Assuming 'distinctSensors' was a typo and 'sensors' should be used.
		"ActiveTab": "report",
		"Logo":      GetCompanyLogo(),
	})
}
