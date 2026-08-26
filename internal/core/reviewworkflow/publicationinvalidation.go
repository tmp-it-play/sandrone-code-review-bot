package reviewworkflow

import (
	"strings"
	"time"
)

const PublicationInvalidationFenceDelay = 2 * time.Minute

type PublicationInvalidation struct {
	ID             uint64
	RunID          uint64
	InstallationID int64
	Owner          string
	Repository     string
	Number         int
	Marker         string
	ProgressMarker string
	ProgressOwned  bool
	Reason         string
	Attempts       int
	LastError      string
	NextAttemptAt  time.Time
	LeaseToken     string
	LeaseExpiresAt *time.Time
	ResolvedAt     *time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (p PublicationInvalidation) Markers() []string {
	marker := strings.TrimSpace(p.Marker)
	markers := make([]string, 0, 2)
	if marker != "" {
		markers = append(markers, marker)
	}
	progressMarker := strings.TrimSpace(p.ProgressMarker)
	if !p.ProgressOwned || progressMarker == "" || progressMarker == marker {
		return markers
	}
	if _, valid := ProgressMarkerKey(progressMarker); valid {
		markers = append(markers, progressMarker)
	}
	return markers
}

func (p PublicationInvalidation) ReasonFor(marker string) string {
	reason := strings.TrimSpace(p.Reason)
	progressMarker := strings.TrimSpace(p.ProgressMarker)
	if p.ProgressOwned || marker != strings.TrimSpace(p.Marker) || progressMarker == "" || strings.Contains(reason, progressMarker) {
		return reason
	}
	if _, valid := ProgressMarkerKey(progressMarker); !valid {
		return reason
	}
	return reason + "\n\n" + progressMarker
}

func (p PublicationInvalidation) UpsertsProgressComment(marker string) bool {
	marker = strings.TrimSpace(marker)
	if !p.ProgressOwned || marker == "" || marker != strings.TrimSpace(p.Marker) || marker != strings.TrimSpace(p.ProgressMarker) {
		return false
	}
	_, valid := ProgressMarkerKey(marker)
	return valid
}
