package themes

import "log/slog"

// Telemetry is the per-refresh convergence and identity-churn summary. It
// answers the WP-206 calibration questions ("does MU converge before the cap,
// is K right, are identities stable") from observed refreshes instead of a
// code read. Match scores per theme stay on Identities; MeanMatchScore here is
// the aggregate over inherited themes only.
type Telemetry struct {
	// ReconstructionError is ‖V−WH‖_F/‖V‖_F from the factorization: 0 = the
	// themes explain the positive loadings perfectly, 1 = no better than zero.
	ReconstructionError float64 `json:"reconstructionError"`
	// Iterations is how many multiplicative-update iterations ran before the
	// objective converged or hit the cap.
	Iterations int `json:"iterations"`
	// Minted and Retired count identity churn at this refresh. A steady corpus
	// refreshes with both at zero; persistent nonzero churn on small mutations
	// is the instability signal WP-203's threshold may need retuning for.
	Minted  int `json:"minted"`
	Retired int `json:"retired"`
	// MeanMatchScore averages the predecessor cosine over themes that inherited
	// an ID this refresh (minted themes excluded). Zero when nothing inherited.
	MeanMatchScore float64 `json:"meanMatchScore"`
}

func buildTelemetry(result *Result) Telemetry {
	t := Telemetry{
		ReconstructionError: result.ReconstructionError,
		Iterations:          result.Iterations,
		Minted:              len(result.MintedIDs),
		Retired:             len(result.RetiredIDs),
	}
	minted := make(map[string]bool, len(result.MintedIDs))
	for _, id := range result.MintedIDs {
		minted[id] = true
	}
	sum, inherited := 0.0, 0
	for _, id := range result.Identities {
		if minted[id.StableID] {
			continue
		}
		sum += id.MatchScore
		inherited++
	}
	if inherited > 0 {
		t.MeanMatchScore = sum / float64(inherited)
	}
	return t
}

func logTelemetry(rev uint64, themeCount int, t Telemetry) {
	slog.Info("themes refresh",
		"revision", rev,
		"themes", themeCount,
		"reconstructionError", t.ReconstructionError,
		"iterations", t.Iterations,
		"minted", t.Minted,
		"retired", t.Retired,
		"meanMatchScore", t.MeanMatchScore,
	)
}
