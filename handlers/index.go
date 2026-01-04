package handlers

import (
	"net/http"
	"time"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// Index renders the main form page
func Index(c *gin.Context) {
	// Default code based on current month
	defaultCode := time.Now().Format("FMS Jan 2006")
	defaultDate := time.Now().Format("2006-01-02")

	// Fetch ships for dropdown
	rowsShips, err := db.DB.Query("SELECT id, name FROM fms_ships ORDER BY name ASC")
	var ships []struct {
		ID   int
		Name string
	}
	if err == nil {
		defer rowsShips.Close()
		for rowsShips.Next() {
			var s struct {
				ID   int
				Name string
			}
			if err := rowsShips.Scan(&s.ID, &s.Name); err == nil {
				ships = append(ships, s)
			}
		}
	}

	// Fetch Projects
	rowsProjects, err := db.DB.Query("SELECT code, name FROM fms_projects WHERE is_active = true ORDER BY name ASC")
	var projects []struct {
		Code string
		Name string
	}
	if err == nil {
		defer rowsProjects.Close()
		for rowsProjects.Next() {
			var p struct {
				Code string
				Name string
			}
			if err := rowsProjects.Scan(&p.Code, &p.Name); err == nil {
				projects = append(projects, p)
			}
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"DefaultCode": defaultCode,
		"DefaultDate": defaultDate,
		"Ships":       ships,
		"Projects":    projects,
		"ActiveTab":   "input",
		"Logo":        GetCompanyLogo(),
	})
}
