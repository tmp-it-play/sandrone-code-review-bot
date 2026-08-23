package handlecommand

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

func commandMarker(request Request) string {
	return "<!-- sandrone-command:" + commandOperationKey(request) + " -->"
}

func commandOperationKey(request Request) string {
	identity := request.RequestIdentity
	if request.Command.CommentID > 0 {
		identity = "comment:" + strconv.FormatInt(request.Command.CommentID, 10)
	}
	hash := sha256.New()
	for _, part := range []string{
		strconv.FormatInt(request.Target.InstallationID, 10),
		request.Target.Owner,
		request.Target.Repository,
		strconv.Itoa(request.Target.Number),
		string(request.Command.Kind),
		strconv.FormatBool(request.Command.InThread),
		identity,
	} {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func commandOrderKey(request Request) string {
	scope := "issue"
	if request.Command.InThread {
		scope = "review"
	}
	return fmt.Sprintf("%s:%020d", scope, request.Command.CommentID)
}
