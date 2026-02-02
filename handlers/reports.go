package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// ListReports returns all device reports for a given code/period with pagination
func ListReports(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		code = time.Now().Format("FMS Jan 2006")
	}

	// Get page number
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	// Pagination settings
	perPage := 20
	offset := (page - 1) * perPage

	// Get total count
	var totalRecords int
	err = db.DB.QueryRow(`SELECT COUNT(*) FROM fms_device_reports WHERE code LIKE $1`, code).Scan(&totalRecords)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	// Get paginated data
	rows, err := db.DB.Query(`
		SELECT id, code, report_date, ship_name,
		       device_condition, gps, rpm_me_port, rpm_me_stbd,
		       flowmeter_input, flowmeter_output, flowmeter_bunker,
		       sensors_data,
		       created_at, updated_at
		FROM fms_device_reports
		WHERE code LIKE $1
		ORDER BY report_date ASC, ship_name ASC
		LIMIT $2 OFFSET $3
	`, code, perPage, offset)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}
	defer rows.Close()

	var reports []DeviceReport
	for rows.Next() {
		var r DeviceReport
		if err := rows.Scan(
			&r.ID, &r.Code, &r.ReportDate, &r.ShipName,
			&r.DeviceCondition, &r.GPS, &r.RpmMEPort, &r.RpmMEStbd,
			&r.FlowmeterInput, &r.FlowmeterOutput, &r.FlowmeterBunker,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			continue
		}
		r.CalculateTotals()
		reports = append(reports, r)
	}

	// Calculate pagination info
	totalPages := (totalRecords + perPage - 1) / perPage
	if totalPages < 1 {
		totalPages = 1
	}

	startIndex := offset + 1
	endIndex := offset + len(reports)
	if endIndex > totalRecords {
		endIndex = totalRecords
	}

	prevPage := page - 1
	if prevPage < 1 {
		prevPage = 1
	}

	nextPage := page + 1
	if nextPage > totalPages {
		nextPage = totalPages
	}

	// Generate page numbers (show max 5 pages around current)
	pageNumbers := []int{}
	startPage := page - 2
	if startPage < 1 {
		startPage = 1
	}
	endPage := startPage + 4
	if endPage > totalPages {
		endPage = totalPages
		startPage = endPage - 4
		if startPage < 1 {
			startPage = 1
		}
	}
	for i := startPage; i <= endPage; i++ {
		pageNumbers = append(pageNumbers, i)
	}

	paginationData := PaginationData{
		Reports:      reports,
		CurrentPage:  page,
		TotalPages:   totalPages,
		TotalRecords: totalRecords,
		StartIndex:   startIndex,
		EndIndex:     endIndex,
		PrevPage:     prevPage,
		NextPage:     nextPage,
		PageNumbers:  pageNumbers,
	}

	c.HTML(http.StatusOK, "reports_table.html", paginationData)
}

// CreateReport creates a new device report
func CreateReport(c *gin.Context) {
	code := c.PostForm("code")
	projectCode := c.PostForm("project_code")
	periodStr := c.PostForm("report_period")   // YYYY-MM
	reportDateStr := c.PostForm("report_date") // Restore this!

	// Handle ship selection (ID vs Name)
	shipID := c.PostForm("ship_id")
	shipName := c.PostForm("ship_name") // Fallback or direct input

	var shipCode string
	if shipID != "" {
		var name string
		// Lookup name and code from ID
		if err := db.DB.QueryRow("SELECT name, code FROM fms_ships WHERE id = $1", shipID).Scan(&name, &shipCode); err == nil {
			shipName = name
		}
	}

	// Construct code if empty
	if code == "" && projectCode != "" && periodStr != "" {
		periodDate, err := time.Parse("2006-01", periodStr)
		if err == nil {
			// Format: PROJECT SHIPCODE PERIOD
			code = fmt.Sprintf("%s %s %s", projectCode, shipCode, periodDate.Format("Jan 2006"))
			code = strings.TrimSpace(code)
		}
	}

	if code == "" || reportDateStr == "" || shipName == "" {
		c.String(http.StatusBadRequest, "Code, date, and ship name are required")
		return
	}

	reportDate, err := time.Parse("2006-01-02", reportDateStr)
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid date format")
		return
	}

	// Parse dynamic sensor inputs
	sensorsData := make(map[string]bool)
	form := c.Request.PostForm
	// Legacy values for backward compatibility
	var deviceCondition, gps, rpmMEPort, rpmMEStbd, flowmeterInput, flowmeterOutput, flowmeterBunker bool

	// Map legacy codes to variable pointers
	legacyMap := map[string]*bool{
		"device_condition": &deviceCondition,
		"gps":              &gps,
		"rpm_me_port":      &rpmMEPort,
		"rpm_me_stbd":      &rpmMEStbd,
		"flowmeter_input":  &flowmeterInput,
		"flowmeter_output": &flowmeterOutput,
		"flowmeter_bunker": &flowmeterBunker,
	}

	for key, values := range form {
		if strings.HasPrefix(key, "sensor_") && len(values) > 0 {
			code := strings.TrimPrefix(key, "sensor_")
			isOn := values[0] == "on"
			sensorsData[code] = isOn

			// Populate legacy fields if matching
			if ptr, ok := legacyMap[code]; ok {
				*ptr = isOn
			}
		}
	}

	// Double check legacy specific inputs in case the form was old style (fallback)
	// Although index.html is updated, API calls might differ
	if !sensorsData["device_condition"] && c.PostForm("device_condition") != "" {
		deviceCondition = c.PostForm("device_condition") == "on"
	}

	// Serialize to JSON
	jsonData, _ := json.Marshal(sensorsData)

	var id int
	err = db.DB.QueryRow(`
		INSERT INTO fms_device_reports 
		(code, report_date, ship_name, device_condition, gps, rpm_me_port, rpm_me_stbd, flowmeter_input, flowmeter_output, flowmeter_bunker, sensors_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id
	`, code, reportDate, shipName, deviceCondition, gps, rpmMEPort, rpmMEStbd, flowmeterInput, flowmeterOutput, flowmeterBunker, jsonData).Scan(&id)

	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	// Return the new row as HTML
	r := DeviceReport{
		ID:              id,
		Code:            code,
		ReportDate:      reportDate,
		ShipName:        shipName,
		DeviceCondition: deviceCondition,
		GPS:             gps,
		RpmMEPort:       rpmMEPort,
		RpmMEStbd:       rpmMEStbd,
		FlowmeterInput:  flowmeterInput,
		FlowmeterOutput: flowmeterOutput,
		FlowmeterBunker: flowmeterBunker,
		SensorsData:     sensorsData,
	}
	r.CalculateTotals()

	c.Header("HX-Trigger-After-Swap", `{"showMessage": "Data laporan berhasil ditambahkan! ✅"}`)
	c.HTML(http.StatusOK, "report_row.html", r)
}

// DeleteReport deletes a device report
// DeleteReport deletes a device report
func DeleteReport(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	_, err = db.DB.Exec(`DELETE FROM fms_device_reports WHERE id = $1`, id)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	// Helper: Check if request expects HTML/Redirect (e.g. from a form submit) or just HTMX
	if c.GetHeader("HX-Request") == "" {
		c.Redirect(http.StatusSeeOther, "/report?success=Data+dihapus")
		return
	}

	c.String(http.StatusOK, "")
}

// UpdateReport updates a device report inline
func UpdateReport(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.String(http.StatusBadRequest, "invalid id")
		return
	}

	field := c.PostForm("field")
	value := c.PostForm("value") == "true"

	validFields := map[string]string{
		"device_condition": "device_condition",
		"gps":              "gps",
		"rpm_me_port":      "rpm_me_port",
		"rpm_me_stbd":      "rpm_me_stbd",
		"flowmeter_input":  "flowmeter_input",
		"flowmeter_output": "flowmeter_output",
		"flowmeter_bunker": "flowmeter_bunker",
	}

	dbField, ok := validFields[field]
	if !ok {
		c.String(http.StatusBadRequest, "invalid field")
		return
	}

	query := "UPDATE fms_device_reports SET " + dbField + " = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2"
	_, err = db.DB.Exec(query, value, id)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	c.String(http.StatusOK, "updated")
}

// EditReportPage renders the edit form for a report
func EditReportPage(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	// Fetch Report Data
	var r DeviceReport
	var sensorsDataJSON []byte
	err := db.DB.QueryRow(`
		SELECT id, code, report_date, ship_name,
		       device_condition, gps, rpm_me_port, rpm_me_stbd,
		       flowmeter_input, flowmeter_output, flowmeter_bunker,
		       sensors_data
		FROM fms_device_reports WHERE id = $1`, id).Scan(
		&r.ID, &r.Code, &r.ReportDate, &r.ShipName,
		&r.DeviceCondition, &r.GPS, &r.RpmMEPort, &r.RpmMEStbd,
		&r.FlowmeterInput, &r.FlowmeterOutput, &r.FlowmeterBunker,
		&sensorsDataJSON,
	)
	if err != nil {
		c.String(http.StatusNotFound, "Report not found")
		return
	}

	// Unmarshal sensors data
	json.Unmarshal(sensorsDataJSON, &r.SensorsData)

	// Fetch Ships for dropdown
	rowsShips, _ := db.DB.Query("SELECT id, name FROM fms_ships ORDER BY name ASC")
	var ships []struct {
		ID   int
		Name string
	}
	defer rowsShips.Close()
	for rowsShips.Next() {
		var s struct {
			ID   int
			Name string
		}
		rowsShips.Scan(&s.ID, &s.Name)
		ships = append(ships, s)
	}

	// Fetch Projects
	rowsProjects, _ := db.DB.Query("SELECT code, name FROM fms_projects WHERE is_active = true ORDER BY name ASC")
	var projects []struct {
		Code string
		Name string
	}
	defer rowsProjects.Close()
	for rowsProjects.Next() {
		var p struct {
			Code string
			Name string
		}
		rowsProjects.Scan(&p.Code, &p.Name)
		projects = append(projects, p)
	}

	// Helper to split code to get project code (Code format: PROJECT SHIP PERIOD)
	parts := strings.Split(r.Code, " ")
	projectCode := ""
	if len(parts) > 0 {
		projectCode = parts[0]
	}

	// Fetch Ship ID
	var currentShipID int
	db.DB.QueryRow("SELECT id FROM fms_ships WHERE name = $1", r.ShipName).Scan(&currentShipID)

	// Fetch Sensors
	var sensors []struct {
		Code string
		Name string
		IsOn bool
	}

	if currentShipID != 0 {
		query := `
			SELECT DISTINCT g.code, g.name
			FROM fms_sensor_config g
			LEFT JOIN fms_ship_sensors s ON g.code = s.sensor_code AND s.ship_id = $1
			WHERE 
				(g.is_active = true AND (s.is_active IS NULL OR s.is_active = true)) 
				OR (s.is_active = true)
			ORDER BY g.display_order ASC
		`
		rows, err := db.DB.Query(query, currentShipID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var code, name string
				rows.Scan(&code, &name)
				isOn := false
				if val, ok := r.SensorsData[code]; ok {
					isOn = val
				}
				sensors = append(sensors, struct {
					Code string
					Name string
					IsOn bool
				}{code, name, isOn})
			}
		}
	} else {
		// Fallback
		rows, err := db.DB.Query("SELECT code, name FROM fms_sensor_config WHERE is_active = true ORDER BY display_order ASC")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var code, name string
				rows.Scan(&code, &name)
				isOn := false
				if val, ok := r.SensorsData[code]; ok {
					isOn = val
				}
				sensors = append(sensors, struct {
					Code string
					Name string
					IsOn bool
				}{code, name, isOn})
			}
		}
	}

	// Render
	c.HTML(http.StatusOK, "report_edit.html", gin.H{
		"Report":          r,
		"Ships":           ships,
		"Projects":        projects,
		"ProjectCode":     projectCode,
		"CurrentShipID":   currentShipID,
		"Sensors":         sensors,
		"FormattedDate":   r.ReportDate.Format("2006-01-02"),
		"FormattedPeriod": r.ReportDate.Format("2006-01"),
	})
}

// UpdateReportFull handles the full form update
func UpdateReportFull(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	// Logic similar to CreateReport but UPDATE
	// Note: We might want to re-generate the code if project/ship/period changes?
	// For simplicity, let's keep the code as is or update it if needed.
	// User request didn't specify, but safer to assume they might fix details.

	projectCode := c.PostForm("project_code")
	periodStr := c.PostForm("report_period") // YYYY-MM
	reportDateStr := c.PostForm("report_date")
	shipID := c.PostForm("ship_id")
	shipName := c.PostForm("ship_name")

	// Resolve ship name if ID provided
	var shipCode string
	if shipID != "" {
		var name string
		if err := db.DB.QueryRow("SELECT name, code FROM fms_ships WHERE id = $1", shipID).Scan(&name, &shipCode); err == nil {
			shipName = name
		}
	}

	// Construct New Code
	newCode := ""
	if projectCode != "" && periodStr != "" {
		periodDate, err := time.Parse("2006-01", periodStr)
		if err == nil {
			newCode = fmt.Sprintf("%s %s %s", projectCode, shipCode, periodDate.Format("Jan 2006"))
			newCode = strings.TrimSpace(newCode)
		}
	}

	reportDate, _ := time.Parse("2006-01-02", reportDateStr)

	// Parse Sensors
	sensorsData := make(map[string]bool)
	form := c.Request.PostForm
	// Legacy vars
	var deviceCondition, gps, rpmMEPort, rpmMEStbd, flowmeterInput, flowmeterOutput, flowmeterBunker bool
	legacyMap := map[string]*bool{
		"device_condition": &deviceCondition,
		"gps":              &gps,
		"rpm_me_port":      &rpmMEPort,
		"rpm_me_stbd":      &rpmMEStbd,
		"flowmeter_input":  &flowmeterInput,
		"flowmeter_output": &flowmeterOutput,
		"flowmeter_bunker": &flowmeterBunker,
	}

	for key, values := range form {
		if strings.HasPrefix(key, "sensor_") && len(values) > 0 {
			code := strings.TrimPrefix(key, "sensor_")
			isOn := values[0] == "on"
			sensorsData[code] = isOn
			if ptr, ok := legacyMap[code]; ok {
				*ptr = isOn
			}
		}
	}

	jsonData, _ := json.Marshal(sensorsData)

	// Update Query
	_, err := db.DB.Exec(`
		UPDATE fms_device_reports 
		SET code=$1, report_date=$2, ship_name=$3, 
		    device_condition=$4, gps=$5, rpm_me_port=$6, rpm_me_stbd=$7, 
			flowmeter_input=$8, flowmeter_output=$9, flowmeter_bunker=$10, 
			sensors_data=$11, updated_at=CURRENT_TIMESTAMP
		WHERE id=$12
	`, newCode, reportDate, shipName, deviceCondition, gps, rpmMEPort, rpmMEStbd, flowmeterInput, flowmeterOutput, flowmeterBunker, jsonData, id)

	if err != nil {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/reports/%d/edit?error=Update+failed", id))
		return
	}

	c.Redirect(http.StatusSeeOther, "/report?success=Laporan+berhasil+diupdate!+✅")
}
