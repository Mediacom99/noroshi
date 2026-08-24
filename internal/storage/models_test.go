package storage

import (
	"strings"
	"testing"
	"time"
)

func TestMaintenanceWindowApplies(t *testing.T) {
	// 2026-08-21 is a Friday; derive codes from the dates so the test stays
	// correct regardless of the chosen base date.
	fri := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	sat := fri.AddDate(0, 0, 1)
	friCode := strings.ToLower(fri.Weekday().String()[:3])
	satCode := strings.ToLower(sat.Weekday().String()[:3])
	at := func(d time.Time, h, m int) time.Time {
		return time.Date(d.Year(), d.Month(), d.Day(), h, m, 0, 0, time.UTC)
	}

	tests := []struct {
		name   string
		window MaintenanceWindow
		now    time.Time
		want   bool
	}{
		{"inside same-day window", MaintenanceWindow{Days: "all", StartMinutes: 120, EndMinutes: 240}, at(fri, 3, 0), true},
		{"start is inclusive", MaintenanceWindow{Days: "all", StartMinutes: 120, EndMinutes: 240}, at(fri, 2, 0), true},
		{"end is exclusive", MaintenanceWindow{Days: "all", StartMinutes: 120, EndMinutes: 240}, at(fri, 4, 0), false},
		{"before window", MaintenanceWindow{Days: "all", StartMinutes: 120, EndMinutes: 240}, at(fri, 1, 59), false},
		{"day listed", MaintenanceWindow{Days: friCode, StartMinutes: 120, EndMinutes: 240}, at(fri, 3, 0), true},
		{"day not listed", MaintenanceWindow{Days: satCode, StartMinutes: 120, EndMinutes: 240}, at(fri, 3, 0), false},
		{"day list with several entries", MaintenanceWindow{Days: "mon," + friCode, StartMinutes: 120, EndMinutes: 240}, at(fri, 3, 0), true},
		{"overnight before midnight", MaintenanceWindow{Days: friCode, StartMinutes: 1320, EndMinutes: 120}, at(fri, 23, 0), true},
		{"overnight after midnight belongs to previous day", MaintenanceWindow{Days: friCode, StartMinutes: 1320, EndMinutes: 120}, at(sat, 1, 0), true},
		{"overnight after window ends", MaintenanceWindow{Days: friCode, StartMinutes: 1320, EndMinutes: 120}, at(sat, 3, 0), false},
		{"overnight wrong start day", MaintenanceWindow{Days: satCode, StartMinutes: 1320, EndMinutes: 120}, at(sat, 1, 0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.window.Applies(tt.now); got != tt.want {
				t.Errorf("Applies(%s) = %v, want %v (window %+v)", tt.now, got, tt.want, tt.window)
			}
		})
	}
}
