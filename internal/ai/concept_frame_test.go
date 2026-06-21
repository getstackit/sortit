package ai

import "testing"

func TestConceptFrameIsEmpty(t *testing.T) {
	cases := []struct {
		name  string
		frame ConceptFrame
		want  bool
	}{
		{name: "zero value", frame: ConceptFrame{}, want: true},
		{name: "blank overview only whitespace", frame: ConceptFrame{Overview: "   \n\t"}, want: true},
		{name: "overview set", frame: ConceptFrame{Overview: "Sortit is an issue tracker."}, want: false},
		{name: "concepts only", frame: ConceptFrame{Concepts: []ConceptDigest{{SubjectTag: "ridge regression", Profile: "the ranking model"}}}, want: false},
		{
			name:  "both set",
			frame: ConceptFrame{Overview: "Sortit", Concepts: []ConceptDigest{{SubjectTag: "ridge regression"}}},
			want:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.frame.IsEmpty(); got != tc.want {
				t.Fatalf("IsEmpty() = %v, want %v", got, tc.want)
			}
		})
	}
}
