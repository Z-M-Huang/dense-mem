package handler

import (
	"time"

	"github.com/dense-mem/dense-mem/internal/domain"
)

func factMatchesTemporalWindow(f *domain.Fact, validAt, knownAt *time.Time) bool {
	if f == nil {
		return false
	}
	if validAt != nil {
		if f.ValidFrom != nil && f.ValidFrom.After(*validAt) {
			return false
		}
		if f.ValidTo != nil && !f.ValidTo.After(*validAt) {
			return false
		}
	}
	if knownAt != nil {
		if f.RecordedAt.After(*knownAt) {
			return false
		}
		if f.RecordedTo != nil && !f.RecordedTo.After(*knownAt) {
			return false
		}
	}
	return true
}
