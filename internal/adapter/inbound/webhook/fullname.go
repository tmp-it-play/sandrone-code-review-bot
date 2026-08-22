package webhook

import "strings"

func splitFullName(fullName string) (string, string) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}
