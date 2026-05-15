package service

import "testing"

func TestPSComputeValidityDaysAcceptsPluralUnits(t *testing.T) {
	tests := []struct {
		name string
		days int
		unit string
		want int
	}{
		{name: "days passthrough", days: 1, unit: "days", want: 1},
		{name: "weeks plural", days: 2, unit: "weeks", want: 14},
		{name: "months plural", days: 1, unit: "months", want: 30},
		{name: "singular month still works", days: 3, unit: "month", want: 90},
		{name: "mixed case trimmed", days: 1, unit: " Months ", want: 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := psComputeValidityDays(tt.days, tt.unit); got != tt.want {
				t.Fatalf("psComputeValidityDays(%d, %q) = %d, want %d", tt.days, tt.unit, got, tt.want)
			}
		})
	}
}
