package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// SettingsPage renders the settings page with sensor list
func SettingsPage(c *gin.Context) {
	type SensorRow struct {
		ID           int
		Code         string
		Name         string
		IsActive     bool
		DisplayOrder int
	}

	var sensorRows []SensorRow
	rows, err := db.DB.Query("SELECT id, code, name, is_active, display_order FROM fms_sensor_config ORDER BY display_order ASC, id ASC")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var s SensorRow
			if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.IsActive, &s.DisplayOrder); err == nil {
				sensorRows = append(sensorRows, s)
			}
		}
	}

	c.HTML(http.StatusOK, "settings.html", gin.H{
		"Sensors":       sensorRows,
		"ActiveSidebar": "sensors",
		"ActiveTab":     "settings",
		"Logo":          GetCompanyLogo(),
	})
}

// SettingsProjectsPage renders the project settings page
func SettingsProjectsPage(c *gin.Context) {
	type ProjectRow struct {
		ID       int
		Code     string
		Name     string
		IsActive bool
	}
	var projectRows []ProjectRow
	pRows, err := db.DB.Query("SELECT id, code, name, is_active FROM fms_projects ORDER BY name ASC")
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var p ProjectRow
			if err := pRows.Scan(&p.ID, &p.Code, &p.Name, &p.IsActive); err == nil {
				projectRows = append(projectRows, p)
			}
		}
	}

	c.HTML(http.StatusOK, "settings_projects.html", gin.H{
		"Projects":      projectRows,
		"ActiveSidebar": "projects",
		"ActiveTab":     "settings",
		"Logo":          GetCompanyLogo(),
	})
}

// CreateProject adds a new project
func CreateProject(c *gin.Context) {
	code := c.PostForm("code")
	name := c.PostForm("name")

	if code == "" || name == "" {
		c.Redirect(http.StatusSeeOther, "/settings/projects?error=Code+and+Name+are+required")
		return
	}

	upperCode := strings.ToUpper(strings.TrimSpace(code))

	_, err := db.DB.Exec("INSERT INTO fms_projects (code, name) VALUES ($1, $2)", upperCode, name)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/settings/projects?error=Gagal+menambah+project.+Code+mungkin+sudah+ada.")
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings/projects?success=Project+berhasil+ditambahkan!+✅")
}

// CreateSensor adds a new sensor configuration
func CreateSensor(c *gin.Context) {
	name := c.PostForm("name")

	if name == "" {
		c.String(http.StatusBadRequest, "Name is required")
		return
	}

	// Generate base code: "Engine RPM" -> "engine_rpm"
	baseCode := strings.ToLower(name)
	reg, _ := regexp.Compile("[^a-z0-9]+")
	baseCode = reg.ReplaceAllString(baseCode, "_")
	baseCode = strings.Trim(baseCode, "_")

	// Ensure code is unique
	code := baseCode
	counter := 1
	for {
		var exists bool
		err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM fms_sensor_config WHERE code = $1)", code).Scan(&exists)
		if err != nil || !exists {
			break // Code is unique or error (let insert handle error)
		}
		// If exists, append suffix
		counter++
		code = baseCode + "_" + strconv.Itoa(counter)
	}

	// Default display order = max + 1
	var maxOrder int
	_ = db.DB.QueryRow("SELECT COALESCE(MAX(display_order), 0) FROM fms_sensor_config").Scan(&maxOrder)

	_, err := db.DB.Exec("INSERT INTO fms_sensor_config (code, name, is_active, display_order) VALUES ($1, $2, true, $3)", code, name, maxOrder+1)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	// Redirect back to settings
	c.Redirect(http.StatusSeeOther, "/settings?success=Sensor+berhasil+ditambahkan!+✅")
}

// ToggleSensor toggles the active status of a sensor
func ToggleSensor(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	_, err := db.DB.Exec("UPDATE fms_sensor_config SET is_active = NOT is_active WHERE id = $1", id)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/settings?success=Status+sensor+diupdate!+🔄")
}

// Helper to get company logo
var cachedLogo string

func GetCompanyLogo() string {
	if cachedLogo != "" {
		return cachedLogo
	}
	var val string
	// Check if table exists (migration might run async or manual, better safe)
	err := db.DB.QueryRow("SELECT value FROM fms_app_config WHERE key='company_logo'").Scan(&val)
	if err == nil {
		cachedLogo = val
	}
	// Fallback/Default
	if cachedLogo == "" {
		cachedLogo = "/static/images/logo-placeholder.png"
	}
	return cachedLogo
}

// SettingsGeneralPage renders the general settings (Logo, etc)
func SettingsGeneralPage(c *gin.Context) {
	logo := GetCompanyLogo()
	c.HTML(http.StatusOK, "settings_general.html", gin.H{
		"Logo":          logo,
		"ActiveSidebar": "general",
		"ActiveTab":     "settings",
	})
}

// UpdateLogo handles logo upload
func UpdateLogo(c *gin.Context) {
	file, err := c.FormFile("logo")
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/settings/general?error=No+file+uploaded")
		return
	}

	// Validate extension
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		c.Redirect(http.StatusSeeOther, "/settings/general?error=Only+PNG+or+JPG+allowed")
		return
	}

	// Create dir
	if err := os.MkdirAll("static/images", 0755); err != nil {
		c.Redirect(http.StatusSeeOther, "/settings/general?error=Server+FS+Error")
		return
	}

	// Save file with specific name to avoid pileup
	// We can add timestamp to bust cache in browser if needed, or query param
	filename := fmt.Sprintf("company_logo%s", ext)
	dst := filepath.Join("static", "images", filename)

	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.Redirect(http.StatusSeeOther, "/settings/general?error=Failed+to+save+file")
		return
	}

	// Update DB
	dbPath := fmt.Sprintf("/static/images/%s", filename)
	_, err = db.DB.Exec("INSERT INTO fms_app_config (key, value) VALUES ('company_logo', $1) ON CONFLICT (key) DO UPDATE SET value = $1", dbPath)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/settings/general?error=DB+Update+Error")
		return
	}

	cachedLogo = dbPath // Update cache

	c.Redirect(http.StatusSeeOther, "/settings/general?success=Logo+updated!+✅")
}
