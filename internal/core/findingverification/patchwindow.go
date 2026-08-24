package findingverification

import (
	"fmt"
	"strings"

	"github.com/it-play/sandrone-code-review-bot/internal/core/diff"
)

const verifierPatchWindowChars = 3000
const verifierPatchContextLines = 3

func patchWindow(patch string, startLine int, endLine int) string {
	if endLine <= 0 {
		endLine = startLine
	}
	for _, hunk := range (diff.Parser{}).Parse(patch) {
		if !hunk.Complete {
			continue
		}
		if startLine < hunk.NewStart || endLine >= hunk.NewStart+hunk.NewCount {
			continue
		}
		body := strings.Split(hunk.Body, "\n")
		newLine := hunk.NewStart
		targetStart := -1
		targetEnd := -1
		for index, raw := range body {
			switch {
			case strings.HasPrefix(raw, "+"):
				if newLine >= startLine && newLine <= endLine {
					if targetStart < 0 {
						targetStart = index
					}
					targetEnd = index
				}
				newLine++
			case strings.HasPrefix(raw, " "):
				newLine++
			}
		}
		if targetStart < 0 || targetEnd < targetStart {
			continue
		}
		windowStart := max(0, targetStart-verifierPatchContextLines)
		windowEnd := min(len(body), targetEnd+verifierPatchContextLines+1)
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount, hunk.Section)
		for len(header)+1+len(strings.Join(body[windowStart:windowEnd], "\n")) > verifierPatchWindowChars && (windowStart < targetStart || windowEnd > targetEnd+1) {
			if windowEnd > targetEnd+1 {
				windowEnd--
			}
			if windowStart < targetStart && len(header)+1+len(strings.Join(body[windowStart:windowEnd], "\n")) > verifierPatchWindowChars {
				windowStart++
			}
		}
		return header + "\n" + strings.Join(body[windowStart:windowEnd], "\n")
	}
	return ""
}
