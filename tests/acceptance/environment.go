package acceptance

import "strings"

// environmentValue reads the last occurrence of an environment variable from a
// synthetic environ list. It lives in a non-test file because process_group.go
// uses it when launching isolated acceptance processes.
func environmentValue(environment []string, name string) string {
	prefix := name + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func environmentValues(environment []string, name string) []string {
	prefix := name + "="
	var values []string
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			values = append(values, strings.TrimPrefix(entry, prefix))
		}
	}
	return values
}
