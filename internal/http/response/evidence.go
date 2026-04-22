package response

import (
	"github.com/dense-mem/dense-mem/internal/domain"
	dto "github.com/dense-mem/dense-mem/internal/http/dto"
)

func toEvidenceResponses(items []domain.Evidence) []dto.Evidence {
	if len(items) == 0 {
		return nil
	}
	out := make([]dto.Evidence, 0, len(items))
	for _, item := range items {
		out = append(out, dto.Evidence{
			FragmentID:        item.FragmentID,
			Speaker:           item.Speaker,
			SpanStart:         item.SpanStart,
			SpanEnd:           item.SpanEnd,
			ExtractConf:       item.ExtractConf,
			ExtractionModel:   item.ExtractionModel,
			ExtractionVersion: item.ExtractionVersion,
			PipelineRunID:     item.PipelineRunID,
			Authority:         string(item.Authority),
		})
	}
	return out
}
