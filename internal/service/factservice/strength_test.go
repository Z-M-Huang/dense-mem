package factservice

import (
	"testing"

	"github.com/dense-mem/dense-mem/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestCompareStrength covers AC-35 (OR support-gate semantics feed into strength)
// and AC-38 (strength comparison drives supersession decisions).
func TestCompareStrength(t *testing.T) {
	bornOnGate := DefaultPromotionGates["born_on"] // minSrc=1, minMaxQ=0.95, extract=0.9, res=0.8
	likesGate := DefaultPromotionGates["likes"]    // minSrc=1, minMaxQ=0.00 (disabled)

	t.Run("identical claims produce equal strength", func(t *testing.T) {
		c := &domain.Claim{
			ProfileID:      "profile-A",
			ExtractConf:    0.92,
			ResolutionConf: 0.85,
			SourceQuality:  0.97,
			SupportedBy:    []string{"src-1"},
		}
		strA := ClaimStrength(c, bornOnGate)
		strB := ClaimStrength(c, bornOnGate)
		require.Equal(t, StrengthEqual, CompareStrength(strA, strB),
			"two computations of the same claim must be equal")
	})

	t.Run("claims with different confs are incomparable", func(t *testing.T) {
		cA := &domain.Claim{
			ProfileID: "profile-A", ExtractConf: 0.92, ResolutionConf: 0.85,
			SourceQuality: 0.97, SupportedBy: []string{"src-1"},
		}
		cB := &domain.Claim{
			ProfileID: "profile-A", ExtractConf: 0.80, ResolutionConf: 0.75,
			SourceQuality: 0.97, SupportedBy: []string{"src-1"},
		}
		require.Equal(t, StrengthIncomparable,
			CompareStrength(ClaimStrength(cA, bornOnGate), ClaimStrength(cB, bornOnGate)),
			"claims with different ResolutionConf/ExtractConf must be incomparable")
	})

	t.Run("facts with same gate outcomes compare by stored TruthScore", func(t *testing.T) {
		// Both facts have SourceQuality 0.96 >= 0.95 → maxSourceQualityGateMet=true
		// Both get ResolutionConf=0, ExtractConf=0 → comparison vector equal → comparable
		fStrong := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.85, SourceQuality: 0.96}
		fWeak := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.70, SourceQuality: 0.96}

		result := CompareStrength(FactStrength(fStrong, bornOnGate), FactStrength(fWeak, bornOnGate))
		require.Equal(t, StrengthAGreater, result,
			"fact with higher stored TruthScore must be ranked stronger")
	})

	t.Run("facts with same gate outcomes and equal TruthScore are equal", func(t *testing.T) {
		fA := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.75, SourceQuality: 0.96}
		fB := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.75, SourceQuality: 0.96}

		require.Equal(t, StrengthEqual,
			CompareStrength(FactStrength(fA, bornOnGate), FactStrength(fB, bornOnGate)))
	})

	t.Run("facts with differing maxSourceQualityGateMet are incomparable", func(t *testing.T) {
		// fA meets the quality gate; fB does not
		fA := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.80, SourceQuality: 0.96}
		fB := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.80, SourceQuality: 0.50}

		require.Equal(t, StrengthIncomparable,
			CompareStrength(FactStrength(fA, bornOnGate), FactStrength(fB, bornOnGate)),
			"facts with different maxSourceQualityGateMet must be incomparable")
	})

	t.Run("ClaimStrength truth_score formula — all gates met", func(t *testing.T) {
		// extract=1.0, resolution=1.0, support>=1, quality>=0.95
		// score = 0.35*1 + 0.35*1 + 0.15*1 + 0.15*1 = 1.0
		c := &domain.Claim{
			ProfileID:      "profile-A",
			ExtractConf:    1.0,
			ResolutionConf: 1.0,
			SourceQuality:  0.97,
			SupportedBy:    []string{"src-1"},
		}
		s := ClaimStrength(c, bornOnGate)
		require.True(t, s.SupportCountGateMet)
		require.True(t, s.MaxSourceQualityGateMet)
		require.InDelta(t, 1.0, s.TruthScore, 1e-9,
			"full gate satisfaction with conf=1.0 must yield TruthScore=1.0")
	})

	t.Run("ClaimStrength truth_score formula — no gates met", func(t *testing.T) {
		// extract=0, resolution=0, no support, no quality
		// score = 0.35*0 + 0.35*0 + 0.15*0 + 0.15*0 = 0.0
		c := &domain.Claim{
			ProfileID:      "profile-A",
			ExtractConf:    0.0,
			ResolutionConf: 0.0,
			SourceQuality:  0.0,
			SupportedBy:    nil,
		}
		s := ClaimStrength(c, bornOnGate)
		require.False(t, s.SupportCountGateMet)
		require.False(t, s.MaxSourceQualityGateMet)
		require.InDelta(t, 0.0, s.TruthScore, 1e-9)
	})

	t.Run("maxSourceQualityGateMet disabled when MinMaxSourceQuality=0 (AC-35 OR semantics)", func(t *testing.T) {
		// likes gate has MinMaxSourceQuality=0.0 — quality alternative path disabled
		c := &domain.Claim{
			ProfileID:      "profile-A",
			ExtractConf:    0.72,
			ResolutionConf: 0.62,
			SourceQuality:  0.99, // high quality, but gate threshold = 0 → gate NOT met
			SupportedBy:    []string{"src-1"},
		}
		s := ClaimStrength(c, likesGate)
		require.False(t, s.MaxSourceQualityGateMet,
			"maxSourceQualityGateMet must be false when gate.MinMaxSourceQuality=0")
		require.True(t, s.SupportCountGateMet)
	})

	t.Run("B greater when second argument has higher TruthScore", func(t *testing.T) {
		fWeak := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.60, SourceQuality: 0.96}
		fStrong := &domain.Fact{ProfileID: "profile-A", TruthScore: 0.80, SourceQuality: 0.96}

		require.Equal(t, StrengthBGreater,
			CompareStrength(FactStrength(fWeak, bornOnGate), FactStrength(fStrong, bornOnGate)))
	})
}

// TestCompareStrength_CrossProfileIsolation verifies that ClaimStrength and
// FactStrength are pure functions that derive all output solely from their
// explicit arguments. Profile A's evidence cannot influence strength results
// computed for profile B.
func TestCompareStrength_CrossProfileIsolation(t *testing.T) {
	gate := DefaultPromotionGates["born_on"]

	// Profile A: high-confidence claim
	claimA := &domain.Claim{
		ProfileID:      "profile-a-550e8400-e29b-41d4-a716-446655440000",
		ExtractConf:    0.95,
		ResolutionConf: 0.90,
		SourceQuality:  0.97,
		SupportedBy:    []string{"src-A-1", "src-A-2"},
	}
	// Profile B: lower-confidence claim with different confs
	claimB := &domain.Claim{
		ProfileID:      "profile-b-6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		ExtractConf:    0.91,
		ResolutionConf: 0.82,
		SourceQuality:  0.96,
		SupportedBy:    []string{"src-B-1"},
	}

	strA := ClaimStrength(claimA, gate)
	strB := ClaimStrength(claimB, gate)

	// Profile A's strength must reflect only claimA's fields.
	require.InDelta(t, claimA.ExtractConf, strA.ExtractConf, 1e-9,
		"strA.ExtractConf must equal claimA.ExtractConf — no cross-profile contamination")
	require.InDelta(t, claimA.ResolutionConf, strA.ResolutionConf, 1e-9,
		"strA.ResolutionConf must equal claimA.ResolutionConf")

	// Profile B's strength must reflect only claimB's fields.
	require.InDelta(t, claimB.ExtractConf, strB.ExtractConf, 1e-9,
		"strB.ExtractConf must equal claimB.ExtractConf — not profile A's value")
	require.InDelta(t, claimB.ResolutionConf, strB.ResolutionConf, 1e-9,
		"strB.ResolutionConf must equal claimB.ResolutionConf")

	// The two strengths must differ — if they were equal it would indicate
	// profile A's data bled into profile B's result.
	require.NotEqual(t, strA.TruthScore, strB.TruthScore,
		"distinct claim confs must produce distinct TruthScores")

	// Claims from different profiles with different confs are incomparable.
	// CompareStrength must not attempt to order them.
	require.Equal(t, StrengthIncomparable, CompareStrength(strA, strB),
		"claims from different profiles with different confs must be incomparable")

	// Fact-level isolation: profile A's stored TruthScore must not appear
	// in profile B's FactStrength result.
	factA := &domain.Fact{
		ProfileID:    "profile-a-550e8400-e29b-41d4-a716-446655440000",
		TruthScore:   0.88,
		SourceQuality: 0.97,
	}
	factB := &domain.Fact{
		ProfileID:    "profile-b-6ba7b810-9dad-11d1-80b4-00c04fd430c8",
		TruthScore:   0.72,
		SourceQuality: 0.96,
	}

	fsA := FactStrength(factA, gate)
	fsB := FactStrength(factB, gate)

	require.InDelta(t, factA.TruthScore, fsA.TruthScore, 1e-9,
		"FactStrength for profile A must use factA.TruthScore")
	require.InDelta(t, factB.TruthScore, fsB.TruthScore, 1e-9,
		"FactStrength for profile B must use factB.TruthScore — not profile A's")
	require.NotContains(t, []float64{fsB.TruthScore}, factA.TruthScore,
		"profile A's TruthScore must not appear in profile B's FactStrength")
}
