package issuemap

// specificityPenalty returns a penalty based on how generic a tag is.
// Lower specificity → higher penalty. When specificity is nil (unscored),
// a default of 0.5 is assumed.
func specificityPenalty(specificity *float64) float64 {
	s := 0.5
	if specificity != nil {
		s = *specificity
	}
	return (1 - s) * 0.04
}
