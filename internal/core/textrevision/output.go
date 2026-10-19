package textrevision

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type Output struct {
	Edits []Edit `json:"edits"`
}

func Parse(content string, fields []Field) ([]Edit, error) {
	var output Output
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("문체 보정 JSON 뒤에 추가 내용이 있습니다")
	}
	return output.Edits, ValidateEdits(fields, output.Edits, true)
}
