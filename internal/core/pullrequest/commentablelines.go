package pullrequest

import (
	"strconv"
	"strings"
)

type CommentableLines struct {
	lines map[int]struct{}
	hunks []Hunk
}

func ParseCommentableLines(patch string) CommentableLines {
	parsed := CommentableLines{lines: map[int]struct{}{}}
	current := 0
	hunkStart := 0
	hunkEnd := 0
	for _, raw := range strings.Split(patch, "\n") {
		if strings.HasPrefix(raw, "@@") {
			if hunkStart > 0 {
				parsed.hunks = append(parsed.hunks, Hunk{Start: hunkStart, End: hunkEnd})
			}
			hunkStart = 0
			hunkEnd = 0
			start, ok := parseHunkStart(raw)
			if !ok {
				current = 0
				continue
			}
			current = start
			continue
		}
		if current == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "+"), strings.HasPrefix(raw, " "), raw == "":
			parsed.lines[current] = struct{}{}
			if hunkStart == 0 {
				hunkStart = current
			}
			hunkEnd = current
			current++
		case strings.HasPrefix(raw, "\\"):
		case strings.HasPrefix(raw, "-"):
		default:
			current++
		}
	}
	if hunkStart > 0 {
		parsed.hunks = append(parsed.hunks, Hunk{Start: hunkStart, End: hunkEnd})
	}
	return parsed
}

func (c CommentableLines) Contains(line int) bool {
	_, ok := c.lines[line]
	return ok
}

func (c CommentableLines) IsEmpty() bool {
	return len(c.lines) == 0
}

func (c CommentableLines) Nearest(line int) (int, bool) {
	for _, hunk := range c.hunks {
		if hunk.Contains(line) {
			return line, true
		}
	}
	best := 0
	bestDistance := -1
	for _, hunk := range c.hunks {
		distance := hunk.Distance(line)
		candidate := hunk.Start
		if line > hunk.End {
			candidate = hunk.End
		}
		if bestDistance < 0 || distance < bestDistance {
			best = candidate
			bestDistance = distance
		}
	}
	if bestDistance < 0 {
		return 0, false
	}
	return best, true
}

func (c CommentableLines) Hunks() []Hunk {
	return c.hunks
}

func parseHunkStart(header string) (int, bool) {
	marker := strings.Index(header, "+")
	if marker < 0 {
		return 0, false
	}
	rest := header[marker+1:]
	end := strings.IndexAny(rest, ", @")
	if end >= 0 {
		rest = rest[:end]
	}
	value, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}
