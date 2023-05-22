package statistic

import (
	"fmt"
	"time"
)

func isWebPageExtension(ext string) bool {
	webPageExts := []string{".html", ".htm", ".php", ".jsp", ".aspx", ".js", ".jsx"}
	for _, e := range webPageExts {
		if e == ext {
			return true
		}
	}
	return false
}

func IsBeforeToday(dateString string) bool {
	layout := "2006_01_02"
	date, err := time.Parse(layout, dateString)
	if err != nil {
		fmt.Println("Error parsing date:", err)
		return false
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	return date.Before(today) || dateString == time.Now().Format(layout)
}
