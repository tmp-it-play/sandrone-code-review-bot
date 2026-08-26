package github

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v90/github"
	"github.com/it-play/sandrone-code-review-bot/internal/core/publication"
)

func withGitHubRetryAt(err error) error {
	if err == nil {
		return nil
	}
	now := time.Now()
	var abuse *gh.AbuseRateLimitError
	if errors.As(err, &abuse) {
		retryAt := retryAfterFromResponse(abuse.Response, now)
		if abuse.RetryAfter != nil && *abuse.RetryAfter > 0 {
			retryAt = now.Add(*abuse.RetryAfter)
		}
		if retryAt.IsZero() {
			retryAt = now.Add(time.Minute)
		}
		return &publication.RetryAtError{At: retryAt, Cause: err}
	}
	var rateLimit *gh.RateLimitError
	if errors.As(err, &rateLimit) {
		retryAt := rateLimit.Rate.Reset.Time
		if !retryAt.After(now) {
			retryAt = rateResetFromResponse(rateLimit.Response, now)
		}
		if retryAt.IsZero() {
			retryAt = now.Add(time.Minute)
		}
		return &publication.RetryAtError{At: retryAt, Cause: err}
	}
	var responseError *gh.ErrorResponse
	if errors.As(err, &responseError) {
		if retryAt := retryAfterFromResponse(responseError.Response, now); !retryAt.IsZero() {
			return &publication.RetryAtError{At: retryAt, Cause: err}
		}
		if responseError.Response != nil && (responseError.Response.StatusCode == http.StatusForbidden || responseError.Response.StatusCode == http.StatusTooManyRequests) {
			if retryAt := rateResetFromResponse(responseError.Response, now); !retryAt.IsZero() {
				return &publication.RetryAtError{At: retryAt, Cause: err}
			}
			message := strings.ToLower(responseError.GetMessage())
			if responseError.Response.StatusCode == http.StatusTooManyRequests || strings.Contains(message, "rate limit") || strings.Contains(message, "abuse") {
				return &publication.RetryAtError{At: now.Add(time.Minute), Cause: err}
			}
		}
	}
	return err
}

func retryAfterFromResponse(response *http.Response, now time.Time) time.Time {
	if response == nil {
		return time.Time{}
	}
	retryAfter := strings.TrimSpace(response.Header.Get("Retry-After"))
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second)
	}
	if parsed, err := http.ParseTime(retryAfter); err == nil && parsed.After(now) {
		return parsed
	}
	return time.Time{}
}

func rateResetFromResponse(response *http.Response, now time.Time) time.Time {
	if response == nil || strings.TrimSpace(response.Header.Get("X-RateLimit-Remaining")) != "0" {
		return time.Time{}
	}
	reset := strings.TrimSpace(response.Header.Get("X-RateLimit-Reset"))
	if seconds, err := strconv.ParseInt(reset, 10, 64); err == nil {
		parsed := time.Unix(seconds, 0)
		if parsed.After(now) {
			return parsed
		}
	}
	return time.Time{}
}
