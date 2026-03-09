package issuemap

type TagRelevance struct {
	Tag       string  `json:"tag"`
	Relevance float64 `json:"relevance"`
}

type Issue struct {
	ID   string
	Raw  string
	Tags []TagRelevance
}
