package hardwareinfo

import (
	"strconv"
	"strings"
)

/*
	Drive Stat parser

	Shared df output parser for linux, darwin and freebsd. This logic used to be
	copy-pasted into each of the three platform files, each with its own
	hard-coded column index and no bounds check, which crashed the whole
	process whenever df produced a line that did not match the expected shape
	(e.g. long device names on QNAP/NAS builds, or a warning line from stderr).

	Kept free of build tags so it stays unit testable on every platform.
*/

// dfMinFields is the smallest number of whitespace separated fields a usable
// df line can have. The POSIX (-P) layout is exactly:
//
//	Filesystem  1024-blocks  Used  Available  Capacity  Mounted-on
//
// BSD's default (non -P) layout adds iused/ifree/%iused before the mount
// point, so it has more fields, never fewer.
const dfMinFields = 6

// dfAvailableField is the index of the "Available" column. This is the 4th
// column in every df layout we support (GNU, macOS and FreeBSD alike), so it
// is safe to address positionally from the front.
const dfAvailableField = 3

// parseDfOutput turns raw `df -k` output into a list of LogicalDisk. It is
// deliberately tolerant: any line it cannot make sense of is skipped rather
// than indexed blindly, so malformed or unexpected output can never panic.
//
// The header line needs no special handling: its Available column is the
// literal text "Available", which fails to parse as a number and is dropped
// by the same guard that drops junk lines.
func parseDfOutput(dfOutput string) []LogicalDisk {
	arr := []LogicalDisk{}

	for _, driveInfo := range strings.Split(dfOutput, "\n") {
		driveInfo = strings.TrimSpace(driveInfo)
		if driveInfo == "" {
			continue
		}

		for strings.Contains(driveInfo, "  ") {
			driveInfo = strings.Replace(driveInfo, "  ", " ", -1)
		}

		driveInfoChunk := strings.Split(driveInfo, " ")
		if len(driveInfoChunk) < dfMinFields {
			// Not a drive line: a wrapped long device name, a df warning that
			// leaked into the output, or otherwise unexpected. Skip it.
			continue
		}

		freeSpaceInKB, err := strconv.Atoi(driveInfoChunk[dfAvailableField])
		if err != nil {
			// The header row, or a pseudo filesystem reporting "-". Skip it.
			continue
		}

		arr = append(arr, LogicalDisk{
			// The mount point is always the last column, whichever layout df
			// used, so address it from the back instead of a fixed index.
			DriveLetter: driveInfoChunk[len(driveInfoChunk)-1],
			FileSystem:  driveInfoChunk[0],
			FreeSpace:   strconv.FormatInt(int64(freeSpaceInKB)*1024, 10), //df show disk space in 1KB blocks
		})
	}

	return arr
}
