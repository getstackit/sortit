package issuemath

import (
	"sort"

	"gonum.org/v1/gonum/mat"

	"sortit/internal/issues"
	"sortit/internal/scoring"
	"sortit/internal/vectors"
)

// LearnTagBasis solves the ranking-regime anchored multi-output ridge problem
// for a corpus's tag vectors:
//
//	T_L = (RᵀR + gamma I)⁻¹ (RᵀE + gamma T⁰)
//
// E must be uncentered, unit-normalized issue embeddings, and R is assembled
// with the same signed-anchor semantics as the ranking ridge. The result is
// normalized row-wise so it remains comparable with descriptor embeddings.
// This is deliberately a pure, offline primitive: no ranking or diagnostic
// consumer calls it until a separately qualified wiring change.
func LearnTagBasis(
	items []issues.Issue,
	tagNames []string,
	issueEmbeddings map[string][]float64,
	descriptorEmbeddings map[string][]float64,
	gamma float64,
) map[string][]float64 {
	dim := learnedTagBasisDim(items, issueEmbeddings, descriptorEmbeddings)
	descriptors := learnedDescriptorRows(tagNames, descriptorEmbeddings, dim)
	if len(items) < scoring.MinDecompositionIssues || len(tagNames) == 0 || dim == 0 || gamma <= 0 {
		return learnedTagBasisMap(tagNames, descriptors)
	}

	tagIndex := make(map[string]int, len(tagNames))
	for i, tag := range tagNames {
		tagIndex[tag] = i
	}

	ordered := append([]issues.Issue(nil), items...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })

	k := len(tagNames)
	a := mat.NewSymDense(k, nil)
	rhs := mat.NewDense(k, dim, nil)
	for row := range k {
		a.SetSym(row, row, gamma)
		for col := range dim {
			rhs.Set(row, col, gamma*descriptors[row][col])
		}
	}

	observed := make([]bool, k)
	valid := 0
	for _, item := range ordered {
		e := issueEmbeddings[item.ID]
		if len(e) != dim || vectors.IsZero(e) {
			continue
		}
		// The ranking regime is uncentered and unit-normalized. Copy before
		// normalizing so this pure helper never mutates caller data.
		e = append([]float64(nil), e...)
		vectors.NormalizeUnit(e)
		anchor, _ := signedAnchor(item.TagScores, tagIndex, k)
		for i, ri := range anchor {
			if ri == 0 {
				continue
			}
			observed[i] = true
			for j := i; j < k; j++ {
				a.SetSym(i, j, a.At(i, j)+ri*anchor[j])
			}
			for d, ed := range e {
				rhs.Set(i, d, rhs.At(i, d)+ri*ed)
			}
		}
		valid++
	}
	if valid < scoring.MinDecompositionIssues {
		return learnedTagBasisMap(tagNames, descriptors)
	}

	var chol mat.Cholesky
	if !chol.Factorize(a) {
		return learnedTagBasisMap(tagNames, descriptors)
	}
	var solved mat.Dense
	if err := chol.SolveTo(&solved, rhs); err != nil {
		return learnedTagBasisMap(tagNames, descriptors)
	}

	learned := make([][]float64, k)
	for i := range k {
		// Preserve the cold-tag invariant exactly, including bit pattern. The
		// descriptor rows used in production are unit vectors, so this is also
		// consistent with the post-solve normalization rule.
		if !observed[i] {
			learned[i] = append([]float64(nil), descriptors[i]...)
			continue
		}
		learned[i] = append([]float64(nil), solved.RawRowView(i)...)
		if !vectors.IsZero(learned[i]) {
			vectors.NormalizeUnit(learned[i])
		}
	}
	return learnedTagBasisMap(tagNames, learned)
}

func learnedTagBasisDim(items []issues.Issue, issueEmbeddings, descriptors map[string][]float64) int {
	for _, item := range items {
		if v := issueEmbeddings[item.ID]; len(v) > 0 {
			return len(v)
		}
	}
	keys := make([]string, 0, len(descriptors))
	for key := range descriptors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if v := descriptors[key]; len(v) > 0 {
			return len(v)
		}
	}
	return 0
}

func learnedDescriptorRows(tagNames []string, descriptors map[string][]float64, dim int) [][]float64 {
	rows := make([][]float64, len(tagNames))
	for i, tag := range tagNames {
		if descriptor := descriptors[tag]; len(descriptor) == dim {
			rows[i] = append([]float64(nil), descriptor...)
		} else {
			rows[i] = make([]float64, dim)
		}
	}
	return rows
}

func learnedTagBasisMap(tagNames []string, rows [][]float64) map[string][]float64 {
	out := make(map[string][]float64, len(tagNames))
	for i, tag := range tagNames {
		out[tag] = append([]float64(nil), rows[i]...)
	}
	return out
}
