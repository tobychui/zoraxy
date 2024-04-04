package utils

import (
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
