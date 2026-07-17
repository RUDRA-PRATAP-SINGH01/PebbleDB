// Package telemetry implements the metric collection, execution timing, and analysis
// aggregation engine for campaign certification runs.
//
// Dependency Rules:
// - Imports: interfaces, types, logging.
package telemetry

import (
	"context"
	"sync"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/types"
)

// MetricRecord aggregates metrics for a single scenario.
type MetricRecord struct {
	Durations map[string][]time.Duration `json:"durations_ms"`
	Counters  map[string]float64         `json:"counters"`
	Gauges    map[string]float64         `json:"gauges"`
}

// TelemetryStore implements interfaces.TelemetryEngine and interfaces.EventSubscriber.
type TelemetryStore struct {
	mu        sync.RWMutex
	records   map[string]*MetricRecord
	startTime time.Time
}

// NewTelemetryStore initializes a thread-safe TelemetryStore.
func NewTelemetryStore() *TelemetryStore {
	return &TelemetryStore{
		records:   make(map[string]*MetricRecord),
		startTime: time.Now(),
	}
}

// Name returns the subscriber registration name.
func (t *TelemetryStore) Name() string {
	return "telemetry_engine"
}

// OnEvent consumes events from the EventBus to log timeline markers.
func (t *TelemetryStore) OnEvent(ctx context.Context, event any) error {
	ev, ok := event.(types.Event)
	if !ok {
		return nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch ev.Type {
	case types.EventSubprocessStarted:
		if session, ok := ev.Payload.(types.ExecutionSession); ok {
			t.ensureRecordLocked(session.ScenarioID)
			t.recordMetricLocked(session.ScenarioID, "subprocess_starts", 1.0, true)
		}
	}
	return nil
}

// RecordDuration writes execution elapsed time for a scenario phase.
func (t *TelemetryStore) RecordDuration(scenarioID string, stage string, elapsed time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	r := t.ensureRecordLocked(scenarioID)
	r.Durations[stage] = append(r.Durations[stage], elapsed)
}

// RecordMetric logs a gauge or incremental counter.
func (t *TelemetryStore) RecordMetric(scenarioID string, name string, value float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.recordMetricLocked(scenarioID, name, value, false)
}

func (t *TelemetryStore) recordMetricLocked(scenarioID string, name string, value float64, increment bool) {
	r := t.ensureRecordLocked(scenarioID)
	if increment {
		r.Counters[name] += value
	} else {
		r.Gauges[name] = value
	}
}

func (t *TelemetryStore) ensureRecordLocked(scenarioID string) *MetricRecord {
	r, exists := t.records[scenarioID]
	if !exists {
		r = &MetricRecord{
			Durations: make(map[string][]time.Duration),
			Counters:  make(map[string]float64),
			Gauges:    make(map[string]float64),
		}
		t.records[scenarioID] = r
	}
	return r
}

// CampaignSummaryReport aggregates execution stats across the campaign.
type CampaignSummaryReport struct {
	ElapsedMs int64                    `json:"elapsed_ms"`
	Scenarios map[string]ScenarioStats `json:"scenarios"`
}

// ScenarioStats aggregates telemetry values for a single scenario.
type ScenarioStats struct {
	Counters      map[string]float64 `json:"counters"`
	Gauges        map[string]float64 `json:"gauges"`
	StageAverages map[string]float64 `json:"stage_averages_ms"`
	StageMaximus  map[string]float64 `json:"stage_maximums_ms"`
}

// Dump compiles and returns the campaign summary metrics.
func (t *TelemetryStore) Dump() interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	report := CampaignSummaryReport{
		ElapsedMs: time.Since(t.startTime).Milliseconds(),
		Scenarios: make(map[string]ScenarioStats),
	}

	for id, record := range t.records {
		stats := ScenarioStats{
			Counters:      make(map[string]float64),
			Gauges:        make(map[string]float64),
			StageAverages: make(map[string]float64),
			StageMaximus:  make(map[string]float64),
		}

		for k, v := range record.Counters {
			stats.Counters[k] = v
		}
		for k, v := range record.Gauges {
			stats.Gauges[k] = v
		}

		for stage, durations := range record.Durations {
			if len(durations) == 0 {
				continue
			}
			var total int64
			var max int64
			for _, d := range durations {
				ms := d.Milliseconds()
				total += ms
				if ms > max {
					max = ms
				}
			}
			stats.StageAverages[stage] = float64(total) / float64(len(durations))
			stats.StageMaximus[stage] = float64(max)
		}

		report.Scenarios[id] = stats
	}

	return report
}
