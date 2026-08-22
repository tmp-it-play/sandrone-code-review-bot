package bootstrap

import (
	"os"
	"strings"
)

const defaultBotName = "sandrone-review-bot"

func botName() string {
	if value := strings.TrimSpace(os.Getenv("SANDRONE_BOT_NAME")); value != "" {
		return value
	}
	return defaultBotName
}
