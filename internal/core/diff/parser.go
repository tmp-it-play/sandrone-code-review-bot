package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeaderPattern = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?: ?(.*))?$`)

type Parser struct{}

func (Parser) Parse(patch string) []Hunk {
	normalized := strings.ReplaceAll(patch, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	hunks := make([]Hunk, 0)
	ordinals := map[string]int{}
	for index := 0; index < len(lines); {
		matches := hunkHeaderPattern.FindStringSubmatch(lines[index])
		if matches == nil {
			index++
			continue
		}
		hunk := Hunk{
			OldStart: parseNumber(matches[1]),
			OldCount: parseCount(matches[2]),
			NewStart: parseNumber(matches[3]),
			NewCount: parseCount(matches[4]),
			Section:  matches[5],
		}
		index++
		body := make([]string, 0)
		oldLines := 0
		newLines := 0
		for index < len(lines) && hunkHeaderPattern.FindStringSubmatch(lines[index]) == nil {
			line := lines[index]
			if index == len(lines)-1 && line == "" {
				index++
				break
			}
			body = append(body, line)
			switch {
			case strings.HasPrefix(line, "+"):
				hunk.AddedLines++
				newLines++
			case strings.HasPrefix(line, "-"):
				hunk.DeletedLines++
				oldLines++
			case strings.HasPrefix(line, " "):
				oldLines++
				newLines++
			}
			index++
		}
		hunk.Body = strings.Join(body, "\n")
		hunk.Hash = hashHunk(hunk.Section, hunk.Body)
		hunk.DuplicateOrdinal = ordinals[hunk.Hash]
		ordinals[hunk.Hash]++
		hunk.Complete = oldLines == hunk.OldCount && newLines == hunk.NewCount
		hunks = append(hunks, hunk)
	}
	return hunks
}

func parseNumber(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

func parseCount(raw string) int {
	if raw == "" {
		return 1
	}
	return parseNumber(raw)
}

func hashHunk(section string, body string) string {
	hash := sha256.New()
	for _, part := range []string{"diff-hunk-v1", section, body} {
		hash.Write([]byte(strconv.Itoa(len(part))))
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
