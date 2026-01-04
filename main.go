package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"fms-app/db"
	"fms-app/handlers"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if err := db.Init(); err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		log.Fatal(err)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// Static
	r.Static("/static", "./static")

	// Templates - load all HTML files
	tmpl := template.Must(template.New("").Funcs(handlers.TemplateFuncs()).ParseGlob("templates/*.html"))
	template.Must(tmpl.ParseGlob("templates/partials/*.html"))
	r.SetHTMLTemplate(tmpl)

	// Routes
	r.GET("/", handlers.Dashboard)
	r.GET("/input", handlers.Index)
	r.GET("/healthz", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// Dashboard (kept for backward compatibility if needed, but root is now preferred)
	r.GET("/dashboard", handlers.Dashboard)
	r.GET("/report", handlers.MonthlyReport)

	// Device Reports API
	r.GET("/reports", handlers.ListReports)
	r.POST("/reports", handlers.CreateReport)
	r.DELETE("/reports/:id", handlers.DeleteReport)
	r.PUT("/reports/:id", handlers.UpdateReport)

	// Rekap
	r.GET("/rekap", handlers.Rekap)

	// JSON API
	r.GET("/api/dashboard-data", handlers.GetDashboardData)
	r.GET("/api/notification-count", handlers.GetNotificationCount)
	r.POST("/api/resolve-alert/:id", handlers.ResolveAlert)

	// Settings
	r.GET("/settings", handlers.SettingsPage)
	r.GET("/settings/projects", handlers.SettingsProjectsPage)
	r.GET("/settings/general", handlers.SettingsGeneralPage)
	r.POST("/settings/logo", handlers.UpdateLogo)
	r.POST("/settings/sensors", handlers.CreateSensor)
	r.POST("/settings/sensors/:id/toggle", handlers.ToggleSensor)
	r.POST("/settings/projects", handlers.CreateProject)

	// Ship Management
	r.GET("/settings/ships", handlers.SettingsShipsPage)
	r.POST("/settings/ships", handlers.CreateShip)
	r.GET("/settings/ships/:id", handlers.SettingsShipConfigPage)
	r.POST("/settings/ships/:id/toggle", handlers.ToggleShipSensor)

	// Batch Input
	r.GET("/batch-input", handlers.BatchInputPage)
	r.POST("/batch-input", handlers.BatchSubmit)

	// HTMX Partial
	r.GET("/api/form-sensors", handlers.FormSensors)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
