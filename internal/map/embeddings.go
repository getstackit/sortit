package issuemap

func Embedding(id string) []float64 {
	return issueDataset().embeddings[id]
}

func AllEmbeddings() map[string][]float64 {
	return issueDataset().embeddings
}

func AllTagEmbeddings() map[string][]float64 {
	return issueDataset().tagEmbeddings
}
