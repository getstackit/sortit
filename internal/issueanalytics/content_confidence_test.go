package issueanalytics

import "testing"

func TestComputeContentConfidenceFeaturesEmptyString(t *testing.T) {
	features := computeContentConfidenceFeatures("")
	if features.Confidence != 0 {
		t.Fatalf("empty confidence = %v, want 0", features.Confidence)
	}
	if features.TokenCount != 0 {
		t.Fatalf("empty token count = %d, want 0", features.TokenCount)
	}
}

func TestComputeContentConfidenceOrdering(t *testing.T) {
	vague := "fix login bug"
	shortSpecific := "Safari export fails after tapping Share twice"
	mediumStructured := "Safari export fails after tapping Share twice.\nExpected: PDF downloads.\nActual: Export hangs."
	detailed := "Safari PDF export fails after tapping Share twice.\n1. Open the Share sheet.\n2. Tap Export as PDF.\n3. Wait 5 seconds.\nError: WebKit export exception in /app/export/pdf.ts:42.\nExpected: a PDF download starts.\nActual: the spinner never clears."
	longRepetitive := "export failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\nexport failed\n"
	structured := "- Open settings\n- Tap export\n- Observe the timeout error in /app/export\n"
	unstructured := "Open settings tap export observe timeout error in app export"

	vagueScore := ComputeContentConfidence(vague)
	shortSpecificScore := ComputeContentConfidence(shortSpecific)
	mediumStructuredScore := ComputeContentConfidence(mediumStructured)
	detailedScore := ComputeContentConfidence(detailed)
	longRepetitiveScore := ComputeContentConfidence(longRepetitive)
	structuredScore := ComputeContentConfidence(structured)
	unstructuredScore := ComputeContentConfidence(unstructured)

	if !(vagueScore < shortSpecificScore && shortSpecificScore < mediumStructuredScore && mediumStructuredScore < detailedScore) {
		t.Fatalf(
			"expected vague < short specific < medium structured < detailed, got vague=%v short=%v medium=%v detailed=%v",
			vagueScore,
			shortSpecificScore,
			mediumStructuredScore,
			detailedScore,
		)
	}
	if detailedScore <= longRepetitiveScore {
		t.Fatalf("expected detailed score %v > repetitive score %v", detailedScore, longRepetitiveScore)
	}
	if structuredScore <= unstructuredScore {
		t.Fatalf("expected structured score %v > unstructured score %v", structuredScore, unstructuredScore)
	}
	if vagueScore >= 0.35 {
		t.Fatalf("expected vague score to stay low, got %v", vagueScore)
	}
	if detailedScore <= 0.75 {
		t.Fatalf("expected detailed score to be high, got %v", detailedScore)
	}
}

func TestComputeContentConfidencePenalizesRepetition(t *testing.T) {
	repetitive := computeContentConfidenceFeatures("timeout error\ntimeout error\ntimeout error\ntimeout error\ntimeout error\ntimeout error")
	informative := computeContentConfidenceFeatures("Timeout error in checkout flow.\n1. Open cart.\n2. Submit payment.\nError: timeout in /checkout/submit.\nExpected: payment succeeds.")

	if repetitive.RepetitionPenalty <= 0 {
		t.Fatalf("expected repetition penalty for repetitive text, got %v", repetitive.RepetitionPenalty)
	}
	if informative.RepetitionPenalty >= repetitive.RepetitionPenalty {
		t.Fatalf(
			"expected informative repetition penalty %v < repetitive penalty %v",
			informative.RepetitionPenalty,
			repetitive.RepetitionPenalty,
		)
	}
	if informative.StructureSignal <= repetitive.StructureSignal {
		t.Fatalf(
			"expected informative structure signal %v > repetitive structure signal %v",
			informative.StructureSignal,
			repetitive.StructureSignal,
		)
	}
}
