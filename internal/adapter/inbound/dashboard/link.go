package dashboard

import (
	"fmt"
	"strconv"
)

func repositoryURL(owner string, name string) string {
	if owner == "" || name == "" {
		return ""
	}
	return "https://github.com/" + owner + "/" + name
}

func pullRequestURL(owner string, name string, number int) string {
	base := repositoryURL(owner, name)
	if base == "" || number <= 0 {
		return ""
	}
	return base + "/pull/" + strconv.Itoa(number)
}

func pullRequestLabel(number int) string {
	if number <= 0 {
		return "-"
	}
	return fmt.Sprintf("#%d", number)
}
