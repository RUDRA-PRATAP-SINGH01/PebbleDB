//go:build go1.26

package db

import "testing"

// reportCustomMetric wraps testing.B.ReportMetric for Go 1.26+ (value, unit).
func reportCustomMetric(b *testing.B, value float64, unit string) {
	b.ReportMetric(value, unit)
}
