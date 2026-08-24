package job

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/pullrequest"
)

func OperationKey(kind string, target pullrequest.Target, requestIdentity string, commentID int64, inThread bool) string {
	identity := strings.TrimSpace(requestIdentity)
	if commentID > 0 {
		identity = "comment:" + strconv.FormatInt(commentID, 10)
	}
	if strings.TrimSpace(kind) == "" || identity == "" {
		return ""
	}
	hash := sha256.New()
	for _, part := range []string{
		strconv.FormatInt(target.InstallationID, 10),
		target.Owner,
		target.Repository,
		strconv.Itoa(target.Number),
		kind,
		strconv.FormatBool(inThread),
		identity,
	} {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func OrderKey(commentID int64, inThread bool) string {
	scope := "issue"
	if inThread {
		scope = "review"
	}
	return fmt.Sprintf("%s:%020d", scope, commentID)
}
