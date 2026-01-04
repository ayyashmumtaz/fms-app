package handlers

import (
	"log"
	"net/http"
	"time"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// Rekap returns the summary/rekap for a given code
func Rekap(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		code = time.Now().Format("FMS Jan 2006")
	}

	// Calculate totals across all ships for this code
	var totalShips, totalOnline, totalOffline int
	err := db.DB.QueryRow(`
		SELECT 
			COUNT(*) as total_ships,
			COALESCE(SUM(
				CASE WHEN device_condition THEN 1 ELSE 0 END +
				CASE WHEN gps THEN 1 ELSE 0 END +
				CASE WHEN rpm_me_port THEN 1 ELSE 0 END +
				CASE WHEN rpm_me_stbd THEN 1 ELSE 0 END +
				CASE WHEN flowmeter_input THEN 1 ELSE 0 END +
				CASE WHEN flowmeter_output THEN 1 ELSE 0 END +
				CASE WHEN flowmeter_bunker THEN 1 ELSE 0 END
			), 0) as total_online,
			COALESCE(SUM(
				CASE WHEN NOT device_condition THEN 1 ELSE 0 END +
				CASE WHEN NOT gps THEN 1 ELSE 0 END +
				CASE WHEN NOT rpm_me_port THEN 1 ELSE 0 END +
				CASE WHEN NOT rpm_me_stbd THEN 1 ELSE 0 END +
				CASE WHEN NOT flowmeter_input THEN 1 ELSE 0 END +
				CASE WHEN NOT flowmeter_output THEN 1 ELSE 0 END +
				CASE WHEN NOT flowmeter_bunker THEN 1 ELSE 0 END
			), 0) as total_offline
		FROM fms_device_reports
		WHERE code = $1
	`, code).Scan(&totalShips, &totalOnline, &totalOffline)

	if err != nil {
		log.Println("REKAP ERROR QUERY:", err)
		c.String(http.StatusInternalServerError, "Error: %v", err)
		return
	}

	totalDevices := totalOnline + totalOffline
	var onlinePercent, offlinePercent float64
	if totalDevices > 0 {
		onlinePercent = float64(totalOnline) / float64(totalDevices) * 100
		offlinePercent = float64(totalOffline) / float64(totalDevices) * 100
	}

	summary := RekapSummary{
		Code:              code,
		TotalShips:        totalShips,
		TotalDevices:      totalDevices,
		TotalOnline:       totalOnline,
		TotalOffline:      totalOffline,
		OnlinePercentage:  onlinePercent,
		OfflinePercentage: offlinePercent,
	}

	c.HTML(http.StatusOK, "rekap.html", summary)
}
