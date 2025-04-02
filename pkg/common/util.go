package common

import "strings"

func Must(e error) {
	if e != nil {
		panic(e)
	}
}

func MustReturn[T any](t T, err error) T {
	Must(err)

	return t
}

func IgnoreError[T any](value T, _ error) T {
	return value
}

func StringToMap(str string) map[string][]string {
	const requiredPartCount = 2

	result := make(map[string][]string)

	for _, line := range strings.Split(str, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", requiredPartCount)
		if len(parts) == requiredPartCount {
			key := strings.TrimSpace(strings.Trim(parts[0], "\" "))
			value := strings.TrimSpace(strings.Trim(parts[1], "\" "))
			result[key] = append(result[key], value)
		}
	}

	return result
}
