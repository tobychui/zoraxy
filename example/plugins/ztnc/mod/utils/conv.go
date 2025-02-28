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
