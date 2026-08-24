package openaicompat

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

type apiError struct {
	Error   json.RawMessage `json:"error"`
	Errors  json.RawMessage `json:"errors"`
	Code    json.RawMessage `json:"code"`
	Message json.RawMessage `json:"message"`
}

func parseAPIError(raw []byte) (string, string) {
	var decoded apiError
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", ""
	}
	return decoded.details()
}

func (e apiError) details() (string, string) {
	code, message := parseErrorDetail(e.Error)
	if code == "" || message == "" {
		cloudflareCode, cloudflareMessage := parseCloudflareErrors(e.Errors)
		if code == "" {
			code = cloudflareCode
		}
		if message == "" {
			message = cloudflareMessage
		}
	}
	if code == "" {
		code = normalizeErrorCode(e.Code)
	}
	if message == "" {
		message = parseErrorMessage(e.Message)
	}
	return code, message
}

func parseCloudflareErrors(raw json.RawMessage) (string, string) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return "", ""
	}
	for _, entry := range entries {
		code, message := parseErrorDetail(entry)
		if code != "" || message != "" {
			return code, message
		}
	}
	return "", ""
}

func parseErrorDetail(raw json.RawMessage) (string, string) {
	var detail struct {
		Code    json.RawMessage `json:"code"`
		Message json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return "", ""
	}
	return normalizeErrorCode(detail.Code), parseErrorMessage(detail.Message)
}

func normalizeErrorCode(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}
	var value string
	if trimmed[0] == '"' {
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return ""
		}
	} else {
		var number json.Number
		if err := json.Unmarshal(trimmed, &number); err != nil {
			return ""
		}
		value = number.String()
	}
	value = strings.TrimSpace(value)
	return truncateUTF8(value, 64)
}

func parseErrorMessage(raw json.RawMessage) string {
	var message string
	if err := json.Unmarshal(raw, &message); err != nil {
		return ""
	}
	return truncateUTF8(strings.TrimSpace(message), 400)
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
