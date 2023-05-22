package utils

import "strconv"

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
