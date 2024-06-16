package dockerux

import "imuslab.com/zoraxy/mod/info/logger"

/*
	Docker Optimizer

	This script add support for optimizing docker user experience
	Note that this module are community contribute only. For bug
	report, please directly tag the Pull Request author.
*/

type UXOptimizer struct {
	RunninInDocker   bool
	SystemWideLogger *logger.Logger
}

//Create a new docker optimizer
func NewDockerOptimizer(IsRunningInDocker bool, logger *logger.Logger) *UXOptimizer {
	return &UXOptimizer{
		RunninInDocker:   IsRunningInDocker,
		SystemWideLogger: logger,
	}
}
