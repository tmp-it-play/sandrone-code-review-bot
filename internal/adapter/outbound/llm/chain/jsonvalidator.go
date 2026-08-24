package chain

import (
	"encoding/json"
	"io"
	"strings"
)

func validJSONObject(content string) bool {
	decoder := json.NewDecoder(strings.NewReader(content))
	var value map[string]json.RawMessage
	if err := decoder.Decode(&value); err != nil || value == nil {
		return false
	}
	var trailing json.RawMessage
	return decoder.Decode(&trailing) == io.EOF
}
