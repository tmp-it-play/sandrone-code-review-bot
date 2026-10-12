package openaicompat

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const maximumRetryAfter = 24 * time.Hour

func providerRetryAfter(header string, raw []byte, now time.Time) time.Duration {
	if duration := parseRetryAfter(header, now); duration > 0 {
		return duration
	}
	for _, entry := range decodeAPIErrors(raw) {
		var detail struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
			} `json:"details"`
		}
		if json.Unmarshal(entry.Error, &detail) != nil {
			continue
		}
		for _, item := range detail.Details {
			if item.Type != "type.googleapis.com/google.rpc.RetryInfo" {
				continue
			}
			if duration, err := time.ParseDuration(item.RetryDelay); err == nil && duration > 0 {
				return min(duration, maximumRetryAfter)
			}
		}
	}
	return 0
}

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
