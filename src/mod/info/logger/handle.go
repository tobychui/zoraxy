package logger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"imuslab.com/zoraxy/mod/utils"
)

// LogConfig represents the log rotation configuration
type LogConfig struct {
	Enabled    bool   `json:"enabled"`    // Whether log rotation is enabled
	MaxSize    string `json:"maxSize"`    // Maximum size as string (e.g., "200M", "10K")
	MaxBackups int    `json:"maxBackups"` // Maximum number of backup files to keep
	Compress   bool   `json:"compress"`   // Whether to compress rotated logs
}

// LoadLogConfig loads the log configuration from the config file
func LoadLogConfig(configPath string) (*LogConfig, error) {
	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}

	// Default config
	defaultConfig := &LogConfig{
		Enabled:    false,
		MaxSize:    "0",
		MaxBackups: 16,
		Compress:   true,
	}

	// Try to read existing config
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, save default config
			if saveErr := SaveLogConfig(configPath, defaultConfig); saveErr != nil {
				return nil, saveErr
			}
			return defaultConfig, nil
		}
		return nil, err
	}
	defer file.Close()

	var config LogConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		// If decode fails, use default
		return defaultConfig, nil
	}

	return &config, nil
}

// SaveLogConfig saves the log configuration to the config file
func SaveLogConfig(configPath string, config *LogConfig) error {
	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(config)
}

// ApplyLogConfig applies the log configuration to the logger
func (l *Logger) ApplyLogConfig(config *LogConfig) error {
	maxSizeBytes, err := utils.SizeStringToBytes(config.MaxSize)
	if err != nil {
		return err
	}

	if maxSizeBytes == 0 {
		// Use default value of 25MB
		maxSizeBytes = 25 * 1024 * 1024
	}

	rotateOption := &RotateOption{
		Enabled:    config.Enabled,
		MaxSize:    int64(maxSizeBytes),
		MaxBackups: config.MaxBackups,
		Compress:   config.Compress,
		BackupDir:  "",
	}

	l.SetRotateOption(rotateOption)
	return nil
}

// HandleGetLogConfig handles GET /api/logger/config
func HandleGetLogConfig(configPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		config, err := LoadLogConfig(configPath)
		if err != nil {
			utils.SendErrorResponse(w, "Failed to load log config: "+err.Error())
			return
		}
		js, err := json.Marshal(config)
		if err != nil {
			utils.SendErrorResponse(w, "Failed to marshal config: "+err.Error())
			return
		}
		utils.SendJSONResponse(w, string(js))
	}
}

// HandleUpdateLogConfig handles POST /api/logger/config
func HandleUpdateLogConfig(configPath string, logger *Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			utils.SendErrorResponse(w, "Method not allowed")
			return
		}

		var config LogConfig
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			utils.SendErrorResponse(w, "Invalid JSON: "+err.Error())
			return
		}

		// Validate MaxSize
		if _, err := utils.SizeStringToBytes(config.MaxSize); err != nil {
			utils.SendErrorResponse(w, "Invalid maxSize: "+err.Error())
			return
		}

		// Validate MaxBackups
		if config.MaxBackups < 1 {
			utils.SendErrorResponse(w, "maxBackups must be at least 1")
			return
		}

		// Save config
		if err := SaveLogConfig(configPath, &config); err != nil {
			utils.SendErrorResponse(w, "Failed to save config: "+err.Error())
			return
		}

		// Apply to logger
		if err := logger.ApplyLogConfig(&config); err != nil {
			utils.SendErrorResponse(w, "Failed to apply config: "+err.Error())
			return
		}

		// Pretty print config as key: value pairs
		configStr := fmt.Sprintf("enabled=%t, maxSize=%s, maxBackups=%d, compress=%t", config.Enabled, config.MaxSize, config.MaxBackups, config.Compress)
		logger.PrintAndLog("logger", "Updated log rotation setting: "+configStr, nil)
		utils.SendOK(w)
	}
}
