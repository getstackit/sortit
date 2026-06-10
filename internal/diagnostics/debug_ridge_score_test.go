package diagnostics

import "testing"

func TestRound3RoundsNegativeValuesAwayFromZero(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0.1234, 0.123},
		{0.1235, 0.124},
		{-0.1234, -0.123},
		{-0.1235, -0.124},
		{-0.0004, 0}, // negative zero compares equal to zero
		{-0.0006, -0.001},
	}
	for _, c := range cases {
		if got := round3(c.in); got != c.want {
			t.Errorf("round3(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
