package issuemap

import "sortit/internal/scoring"

// specificityPenalty returns a penalty based on how generic a tag is.
// Lower specificity → higher penalty. When specificity is nil (unscored),
// a default of GenericTagThreshold is assumed.
func specificityPenalty(specificity *float64) float64 {
	s := scoring.GenericTagThreshold
	if specificity != nil {
		s = *specificity
	}
	return (1 - s) * scoring.SpecificityPenaltyPerTag
}
