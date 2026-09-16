
package hardwareinfo

import "testing"

// TestParseDfOutputMalformedLines is the regression test for the QNAP crash:
// df wrapped a long device name onto its own line, which split into a single
// field and panicked with "index out of range [3] with length 1", taking the
// whole process down from a background goroutine.
func TestParseDfOutputMalformedLines(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t\n  "},
		{"header only", "Filesystem 1024-blocks Used Available Capacity Mounted on"},
		{"single field line", "/dev/mapper/cachedev1"},
		{"wrapped long device name", "/dev/mapper/an-extremely-long-qnap-device-name\n" +
			"                 1922860848 123456 1799404848   6% /share/CACHEDEV1_DATA"},
		{"df warning leaked into output", "df: /mnt/broken: Permission denied"},
		{"truncated row", "/dev/sda1 1024 512"},
		{"non numeric available column", "tmpfs 1024 512 - 0% /run"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The assertion is simply that this returns rather than panics.
			got := parseDfOutput(tc.in)
			if got == nil {
				t.Error("parseDfOutput returned nil, expected an empty slice")
			}
		})
	}
}

// TestParseDfOutputGNULayout verifies the POSIX/GNU six column layout.
func TestParseDfOutputGNULayout(t *testing.T) {
	in := "Filesystem     1024-blocks     Used Available Capacity Mounted on\n" +
		"/dev/sda1         41153856 12345678  26000000      33% /\n" +
		"/dev/sdb1        102400000  1024000 100000000       1% /mnt/data\n"

	got := parseDfOutput(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 drives, got %d: %+v", len(got), got)
	}

	if got[0].FileSystem != "/dev/sda1" {
		t.Errorf("FileSystem = %q, expected /dev/sda1", got[0].FileSystem)
	}
	if got[0].DriveLetter != "/" {
		t.Errorf("DriveLetter = %q, expected /", got[0].DriveLetter)
	}
	// 26000000 KB * 1024 = 26624000000 bytes
	if got[0].FreeSpace != "26624000000" {
		t.Errorf("FreeSpace = %q, expected 26624000000", got[0].FreeSpace)
	}
	if got[1].DriveLetter != "/mnt/data" {
		t.Errorf("DriveLetter = %q, expected /mnt/data", got[1].DriveLetter)
	}
}

// TestParseDfOutputBSDLayout verifies the wider macOS layout, where the mount
// point sits at index 8 rather than 5 because of the extra inode columns.
func TestParseDfOutputBSDLayout(t *testing.T) {
	in := "Filesystem    1024-blocks     Used Available Capacity iused      ifree %iused  Mounted on\n" +
		"/dev/disk1s5s1  971350180 22140900  32873416    41%  502008 4881482672    0%   /\n"

	got := parseDfOutput(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 drive, got %d: %+v", len(got), got)
	}
	if got[0].DriveLetter != "/" {
		t.Errorf("DriveLetter = %q, expected /", got[0].DriveLetter)
	}
	if got[0].FileSystem != "/dev/disk1s5s1" {
		t.Errorf("FileSystem = %q, expected /dev/disk1s5s1", got[0].FileSystem)
	}
	// 32873416 KB * 1024 = 33662377984 bytes
	if got[0].FreeSpace != "33662377984" {
		t.Errorf("FreeSpace = %q, expected 33662377984", got[0].FreeSpace)
	}
}

// TestParseDfOutputMixedGoodAndBad verifies that one unparsable line does not
// discard the drives around it.
func TestParseDfOutputMixedGoodAndBad(t *testing.T) {
	in := "/dev/sda1 41153856 12345678 26000000 33% /\n" +
		"/dev/mapper/a-very-long-name-that-df-wrapped\n" +
		"df: /mnt/broken: Permission denied\n" +
		"/dev/sdb1 102400000 1024000 100000000 1% /mnt/data\n"

	got := parseDfOutput(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 usable drives, got %d: %+v", len(got), got)
	}
}
