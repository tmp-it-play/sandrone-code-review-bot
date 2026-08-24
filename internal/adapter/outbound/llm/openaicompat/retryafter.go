package openaicompat

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maximumRetryAfter = 24 * time.Hour

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		if seconds == 0 {
			return 0
		}
		if seconds >= uint64(maximumRetryAfter/time.Second) {
			return maximumRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	duration := retryAt.Sub(now)
	if duration <= 0 {
		return 0
	}
	if duration > maximumRetryAfter {
		return maximumRetryAfter
	}
	return duration
}
