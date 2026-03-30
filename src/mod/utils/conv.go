package utils

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func StringToInt64(number string) (int64, error) {
	i, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return -1, err
	}
	return i, nil
}

func Int64ToString(number int64) string {
	convedNumber := strconv.FormatInt(number, 10)
	return convedNumber
}

func SizeStringToBytes(sizeStr string) (int64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if len(sizeStr) == 0 {
		return 0, nil
	}
	// Extract unit (1 or 2 characters) from the end of the string
	var unit string
	var sizeValue string

	sizeStrLower := strings.ToLower(sizeStr)
	if len(sizeStrLower) > 2 && (strings.HasSuffix(sizeStrLower, "kb") || strings.HasSuffix(sizeStrLower, "mb") || strings.HasSuffix(sizeStrLower, "gb") || strings.HasSuffix(sizeStrLower, "tb") || strings.HasSuffix(sizeStrLower, "pb")) {
		unit = sizeStrLower[len(sizeStrLower)-2:]
		sizeValue = sizeStrLower[:len(sizeStrLower)-2]
	} else if len(sizeStrLower) > 1 && (strings.HasSuffix(sizeStrLower, "k") || strings.HasSuffix(sizeStrLower, "m") || strings.HasSuffix(sizeStrLower, "g") || strings.HasSuffix(sizeStrLower, "t") || strings.HasSuffix(sizeStrLower, "p")) {
		unit = sizeStrLower[len(sizeStrLower)-1:]
		sizeValue = sizeStrLower[:len(sizeStrLower)-1]
	} else {
		unit = ""
		sizeValue = sizeStrLower
	}

	size, err := strconv.ParseFloat(sizeValue, 64)
	if err != nil {
		return 0, err
	}
	switch unit {
	case "k", "kb":
		size *= 1024
	case "m", "mb":
		size *= 1024 * 1024
	case "g", "gb":
		size *= 1024 * 1024 * 1024
	case "t", "tb":
		size *= 1024 * 1024 * 1024 * 1024
	case "p", "pb":
		size *= 1024 * 1024 * 1024 * 1024 * 1024
	case "":
		// No unit, size is already in bytes
	default:
		return 0, nil // Unknown unit
	}
	return int64(size), nil
}

func BytesToHumanReadable(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return strconv.FormatFloat(float64(bytes)/float64(TB), 'f', 2, 64) + " TB"
	case bytes >= GB:
		return strconv.FormatFloat(float64(bytes)/float64(GB), 'f', 2, 64) + " GB"
	case bytes >= MB:
		return strconv.FormatFloat(float64(bytes)/float64(MB), 'f', 2, 64) + " MB"
	case bytes >= KB:
		return strconv.FormatFloat(float64(bytes)/float64(KB), 'f', 2, 64) + " KB"
	default:
		return strconv.FormatInt(bytes, 10) + " Bytes"
	}
}

func ReplaceSpecialCharacters(filename string) string {
	replacements := map[string]string{
		"#":  "%pound%",
		"&":  "%amp%",
		"{":  "%left_cur%",
		"}":  "%right_cur%",
		"\\": "%backslash%",
		"<":  "%left_ang%",
		">":  "%right_ang%",
		"*":  "%aster%",
		"?":  "%quest%",
		" ":  "%space%",
		"$":  "%dollar%",
		"!":  "%exclan%",
		"'":  "%sin_q%",
		"\"": "%dou_q%",
		":":  "%colon%",
		"@":  "%at%",
		"+":  "%plus%",
		"`":  "%backtick%",
		"|":  "%pipe%",
		"=":  "%equal%",
		".":  "_",
		"/":  "-",
	}

	for char, replacement := range replacements {
		filename = strings.ReplaceAll(filename, char, replacement)
	}

	return filename
}

/* Zip File Handler */
// zipFiles compresses multiple files into a single zip archive file
func ZipFiles(filename string, files ...string) error {
	newZipFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	for _, file := range files {
		if err := addFileToZip(zipWriter, file); err != nil {
			return err
		}
	}
	return nil
}

// addFileToZip adds an individual file to a zip archive
func addFileToZip(zipWriter *zip.Writer, filename string) error {
	fileToZip, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fileToZip.Close()

	info, err := fileToZip.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	header.Name = filepath.Base(filename)
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, fileToZip)
	return err
}
