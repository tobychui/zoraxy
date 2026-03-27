package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SyncStatus struct {
	Mode                         string           `json:"mode"`
	PrimaryServer                string           `json:"primary_server,omitempty"`
	HasPreviousSync              bool             `json:"has_previous_sync"`
	LastAttemptAt                time.Time        `json:"last_attempt_at,omitempty"`
	LastSuccessAt                time.Time        `json:"last_success_at,omitempty"`
	LastConfigVersion            string           `json:"last_config_version,omitempty"`
	LastError                    string           `json:"last_error,omitempty"`
	LocalOverride                bool             `json:"local_override,omitempty"`
	TelemetrySeeded              bool             `json:"telemetry_seeded,omitempty"`
	LastTelemetryAnalyticsDate   string           `json:"last_telemetry_analytics_date,omitempty"`
	LastTelemetryUptimeTimestamp map[string]int64 `json:"last_telemetry_uptime_timestamp,omitempty"`
}

func (s *SyncStatus) normalize() {
	if s == nil {
		return
	}

	if s.HasPreviousSync {
		return
	}

	if !s.LastSuccessAt.IsZero() || strings.TrimSpace(s.LastConfigVersion) != "" {
		s.HasPreviousSync = true
	}

	if s.LastTelemetryUptimeTimestamp == nil {
		s.LastTelemetryUptimeTimestamp = map[string]int64{}
	}
}

func LoadSyncStatus(filename string) (*SyncStatus, error) {
	status := &SyncStatus{}
	content, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(content, status); err != nil {
		return nil, err
	}

	status.normalize()

	return status, nil
}

func SaveSyncStatus(filename string, status *SyncStatus) error {
	if status == nil {
		status = &SyncStatus{}
	}
	status.normalize()

	if err := os.MkdirAll(filepath.Dir(filename), 0775); err != nil {
		return err
	}

	content, err := json.MarshalIndent(status, "", " ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, content, 0775)
}
