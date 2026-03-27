package dockerux

import (
	"sync"

	"imuslab.com/zoraxy/mod/info/logger"
)

/*
	Docker Optimizer

	This script add support for optimizing docker user experience
	Note that this module are community contribute only. For bug
	report, please directly tag the Pull Request author.
*/

type UXOptimizer struct {
	RunninInDocker   bool
	SystemWideLogger *logger.Logger
	imageDetectMutex sync.Mutex
	detectedImage    string
	imageChecked     bool
}

// Create a new docker optimizer
func NewDockerOptimizer(IsRunningInDocker bool, logger *logger.Logger) *UXOptimizer {
	return &UXOptimizer{
		RunninInDocker:   IsRunningInDocker,
		SystemWideLogger: logger,
	}
}
