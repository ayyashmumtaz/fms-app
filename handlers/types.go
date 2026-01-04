package handlers

import "time"

// SensorConfig represents a sensor configuration
type SensorConfig struct {
	Code         string
	Name         string
	IsActive     bool
	DisplayOrder int
}

// DeviceReport represents a single ship's device status report
type DeviceReport struct {
	ID         int
	Code       string
	ReportDate time.Time
	ShipName   string

	// Dynamic Sensor Data
	SensorsData map[string]bool

	// Legacy fields (kept for template compatibility for now, populated from SensorsData)
	DeviceCondition bool
	GPS             bool
	RpmMEPort       bool
	RpmMEStbd       bool
	FlowmeterInput  bool
	FlowmeterOutput bool
	FlowmeterBunker bool

	OnlineTotal    int
	OfflineTotal   int
	OnlinePercent  float64
	OfflinePercent float64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RekapSummary represents the summary/rekap for a period
type RekapSummary struct {
	Code              string
	TotalShips        int
	TotalDevices      int
	TotalOnline       int
	TotalOffline      int
	OnlinePercentage  float64
	OfflinePercentage float64
}

// CalculateTotals calculates online/offline totals dynamically
func (d *DeviceReport) CalculateTotals() {
	d.OnlineTotal = 0
	d.OfflineTotal = 0

	// Use SensorsData map if available (Dynamic Mode)
	if len(d.SensorsData) > 0 {
		for _, status := range d.SensorsData {
			if status {
				d.OnlineTotal++
			} else {
				d.OfflineTotal++
			}
		}
	} else {
		// Fallback to Legacy Fields
		if d.DeviceCondition {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
		if d.GPS {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
		if d.RpmMEPort {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
		if d.RpmMEStbd {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
		if d.FlowmeterInput {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
		if d.FlowmeterOutput {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
		if d.FlowmeterBunker {
			d.OnlineTotal++
		} else {
			d.OfflineTotal++
		}
	}

	total := float64(d.OnlineTotal + d.OfflineTotal)
	if total > 0 {
		d.OnlinePercent = float64(d.OnlineTotal) / total * 100
		d.OfflinePercent = float64(d.OfflineTotal) / total * 100
	}
}

// PaginationData represents pagination information
type PaginationData struct {
	Reports      []DeviceReport
	CurrentPage  int
	TotalPages   int
	TotalRecords int
	StartIndex   int
	EndIndex     int
	PrevPage     int
	NextPage     int
	PageNumbers  []int
}
