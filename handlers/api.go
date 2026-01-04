package handlers

import (
	"net/http"

	"fms-app/db"

	"github.com/gin-gonic/gin"
)

// DashboardDataResponse represents the JSON response for dashboard data
type DashboardDataResponse struct {
	Labels             []string  `json:"labels"`
	OnlinePercentages  []float64 `json:"onlinePercentages"`
	OfflinePercentages []float64 `json:"offlinePercentages"`
	TotalOnline        []int     `json:"totalOnline"`
	TotalOffline       []int     `json:"totalOffline"`
}

// GetDashboardData returns dashboard data as JSON
func GetDashboardData(c *gin.Context) {
	// Get all unique codes with their summaries
	rows, err := db.DB.Query(`
		SELECT 
			code,
			COUNT(*) as total_ships,
			SUM(CASE WHEN device_condition THEN 1 ELSE 0 END) +
			SUM(CASE WHEN gps THEN 1 ELSE 0 END) +
			SUM(CASE WHEN rpm_me_port THEN 1 ELSE 0 END) +
			SUM(CASE WHEN rpm_me_stbd THEN 1 ELSE 0 END) +
			SUM(CASE WHEN flowmeter_input THEN 1 ELSE 0 END) +
			SUM(CASE WHEN flowmeter_output THEN 1 ELSE 0 END) +
			SUM(CASE WHEN flowmeter_bunker THEN 1 ELSE 0 END) as total_online,
			(COUNT(*) * 7) - (
				SUM(CASE WHEN device_condition THEN 1 ELSE 0 END) +
				SUM(CASE WHEN gps THEN 1 ELSE 0 END) +
				SUM(CASE WHEN rpm_me_port THEN 1 ELSE 0 END) +
				SUM(CASE WHEN rpm_me_stbd THEN 1 ELSE 0 END) +
				SUM(CASE WHEN flowmeter_input THEN 1 ELSE 0 END) +
				SUM(CASE WHEN flowmeter_output THEN 1 ELSE 0 END) +
				SUM(CASE WHEN flowmeter_bunker THEN 1 ELSE 0 END)
			) as total_offline
		FROM fms_device_reports
		GROUP BY code
		ORDER BY code DESC
		LIMIT 10
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var labels []string
	var onlinePercentages []float64
	var offlinePercentages []float64
	var totalOnline []int
	var totalOffline []int

	for rows.Next() {
		var code string
		var totalShips, online, offline int

		if err := rows.Scan(&code, &totalShips, &online, &offline); err != nil {
			continue
		}

		total := float64(online + offline)
		var onlinePct, offlinePct float64
		if total > 0 {
			onlinePct = float64(online) / total * 100
			offlinePct = float64(offline) / total * 100
		}

		labels = append(labels, code)
		onlinePercentages = append(onlinePercentages, onlinePct)
		offlinePercentages = append(offlinePercentages, offlinePct)
		totalOnline = append(totalOnline, online)
		totalOffline = append(totalOffline, offline)
	}

	response := DashboardDataResponse{
		Labels:             labels,
		OnlinePercentages:  onlinePercentages,
		OfflinePercentages: offlinePercentages,
		TotalOnline:        totalOnline,
		TotalOffline:       totalOffline,
	}

	c.JSON(http.StatusOK, response)
}
