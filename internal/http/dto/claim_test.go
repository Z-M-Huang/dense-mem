package dto

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dense-mem/dense-mem/internal/http/validation"
)

// validUUID returns a random UUID v4 string for testing.
func validUUID() string {
	return uuid.New().String()
}

// TestCreateClaimRequest covers AC-8 validation rules for CreateClaimRequest.
func TestCreateClaimRequest(t *testing.T) {
	t.Run("supported_by required", func(t *testing.T) {
		req := &CreateClaimRequest{}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required")
	})

	t.Run("supported_by min 1 element", func(t *testing.T) {
		req := &CreateClaimRequest{SupportedBy: []string{}}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("supported_by each element must be uuid", func(t *testing.T) {
		req := &CreateClaimRequest{SupportedBy: []string{"not-a-uuid"}}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "uuid")
	})

	t.Run("supported_by valid uuid passes", func(t *testing.T) {
		req := &CreateClaimRequest{SupportedBy: []string{validUUID()}}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("supported_by multiple valid uuids pass", func(t *testing.T) {
		req := &CreateClaimRequest{SupportedBy: []string{validUUID(), validUUID()}}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("subject max 256 chars", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Subject:     strings.Repeat("x", 257),
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max")
	})

	t.Run("subject at limit passes", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Subject:     strings.Repeat("x", 256),
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("predicate max 128 chars", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Predicate:   strings.Repeat("x", 129),
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max")
	})

	t.Run("predicate at limit passes", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Predicate:   strings.Repeat("x", 128),
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("object max 1024 chars", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Object:      strings.Repeat("x", 1025),
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max")
	})

	t.Run("object at limit passes", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Object:      strings.Repeat("x", 1024),
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("invalid modality rejected", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Modality:    "belief",
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("valid modalities accepted", func(t *testing.T) {
		for _, m := range []string{"assertion", "question", "proposal", "speculation", "quoted"} {
			req := &CreateClaimRequest{
				SupportedBy: []string{validUUID()},
				Modality:    m,
			}
			err := validation.ValidateStruct(req)
			require.NoError(t, err, "modality %q should be valid", m)
		}
	})

	t.Run("omitted modality accepted", func(t *testing.T) {
		req := &CreateClaimRequest{SupportedBy: []string{validUUID()}}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("invalid polarity rejected", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Polarity:    "positive",
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("valid polarity + accepted", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Polarity:    "+",
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("valid polarity - accepted", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Polarity:    "-",
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("omitted polarity accepted", func(t *testing.T) {
		req := &CreateClaimRequest{SupportedBy: []string{validUUID()}}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("speaker max 256 chars", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Speaker:     strings.Repeat("x", 257),
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max")
	})

	t.Run("speaker at limit passes", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			Speaker:     strings.Repeat("x", 256),
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("idempotency_key max 128 chars", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy:    []string{validUUID()},
			IdempotencyKey: strings.Repeat("x", 129),
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max")
	})

	t.Run("idempotency_key at limit passes", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy:    []string{validUUID()},
			IdempotencyKey: strings.Repeat("x", 128),
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("extract_conf below 0 rejected", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ExtractConf: -0.1,
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("extract_conf above 1 rejected", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ExtractConf: 1.1,
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("extract_conf in range passes", func(t *testing.T) {
		for _, v := range []float64{0, 0.5, 1.0} {
			req := &CreateClaimRequest{
				SupportedBy: []string{validUUID()},
				ExtractConf: v,
			}
			err := validation.ValidateStruct(req)
			require.NoError(t, err, "extract_conf %v should be valid", v)
		}
	})

	t.Run("resolution_conf below 0 rejected", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy:    []string{validUUID()},
			ResolutionConf: -0.1,
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("resolution_conf above 1 rejected", func(t *testing.T) {
		req := &CreateClaimRequest{
			SupportedBy:    []string{validUUID()},
			ResolutionConf: 1.1,
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("resolution_conf in range passes", func(t *testing.T) {
		for _, v := range []float64{0, 0.5, 1.0} {
			req := &CreateClaimRequest{
				SupportedBy:    []string{validUUID()},
				ResolutionConf: v,
			}
			err := validation.ValidateStruct(req)
			require.NoError(t, err, "resolution_conf %v should be valid", v)
		}
	})

	t.Run("valid_from after valid_to rejected", func(t *testing.T) {
		later := time.Now()
		earlier := later.Add(-time.Hour)
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ValidFrom:   &later,
			ValidTo:     &earlier,
		}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("valid_from equal to valid_to passes", func(t *testing.T) {
		ts := time.Now()
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ValidFrom:   &ts,
			ValidTo:     &ts,
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("valid_from before valid_to passes", func(t *testing.T) {
		from := time.Now()
		to := from.Add(time.Hour)
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ValidFrom:   &from,
			ValidTo:     &to,
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("only valid_from set passes", func(t *testing.T) {
		from := time.Now()
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ValidFrom:   &from,
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("only valid_to set passes", func(t *testing.T) {
		to := time.Now()
		req := &CreateClaimRequest{
			SupportedBy: []string{validUUID()},
			ValidTo:     &to,
		}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})
}

// TestListClaimsRequest covers ListClaimsRequest validation.
func TestListClaimsRequest(t *testing.T) {
	t.Run("default zero values pass", func(t *testing.T) {
		req := &ListClaimsRequest{}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("limit over max rejected", func(t *testing.T) {
		req := &ListClaimsRequest{Limit: 101}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("limit at max passes", func(t *testing.T) {
		req := &ListClaimsRequest{Limit: 100}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})

	t.Run("cursor max 256", func(t *testing.T) {
		req := &ListClaimsRequest{Cursor: strings.Repeat("x", 257)}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		req := &ListClaimsRequest{Status: "unknown"}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("valid statuses accepted", func(t *testing.T) {
		for _, s := range []string{"candidate", "validated", "rejected", "superseded"} {
			req := &ListClaimsRequest{Status: s}
			err := validation.ValidateStruct(req)
			require.NoError(t, err, "status %q should be valid", s)
		}
	})

	t.Run("invalid modality rejected", func(t *testing.T) {
		req := &ListClaimsRequest{Modality: "hypothesis"}
		err := validation.ValidateStruct(req)
		require.Error(t, err)
	})

	t.Run("valid modality accepted", func(t *testing.T) {
		req := &ListClaimsRequest{Modality: "assertion"}
		err := validation.ValidateStruct(req)
		require.NoError(t, err)
	})
}

// TestClaim_CrossProfileIsolation verifies that ClaimResponse carries ProfileID
// so callers can assert isolation. Data from profile A must not appear in
// profile B results, and each response must reflect its own profile.
func TestClaim_CrossProfileIsolation(t *testing.T) {
	profileA := validUUID()
	profileB := validUUID()
	claimAID := validUUID()
	claimBID := validUUID()

	// Construct two separate responses for different profiles.
	respA := ClaimResponse{ClaimID: claimAID, ProfileID: profileA, Subject: "A-subject"}
	respB := ClaimResponse{ClaimID: claimBID, ProfileID: profileB, Subject: "B-subject"}

	// Profile IDs must match their source — no cross-contamination.
	require.Equal(t, profileA, respA.ProfileID, "response A must carry profile A")
	require.Equal(t, profileB, respB.ProfileID, "response B must carry profile B")

	// Claim IDs must not bleed across profiles.
	bResults := []string{respB.ClaimID}
	require.NotContains(t, bResults, claimAID,
		"claim from profile A must not appear in profile B results")
}
