package job

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

func PublicationMarker(kind string, target pullrequest.Target, requestIdentity string, commentID int64, inThread bool) string {
	identity := strings.TrimSpace(requestIdentity)
	if identity == "" && commentID > 0 {
		scope := "issue"
		if inThread {
			scope = "review"
		}
		identity = scope + ":comment:" + strconv.FormatInt(commentID, 10)
	}
	if strings.TrimSpace(kind) == "" || identity == "" {
		return ""
	}
	hash := sha256.New()
	for _, part := range []string{
		kind,
		strconv.FormatInt(target.InstallationID, 10),
		target.Owner,
		target.Repository,
		strconv.Itoa(target.Number),
		identity,
	} {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return "<!-- sandrone-job:" + hex.EncodeToString(hash.Sum(nil)) + " -->"
}
