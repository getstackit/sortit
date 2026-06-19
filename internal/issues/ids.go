package issues

import (
	"crypto/rand"

	"github.com/oklog/ulid/v2"
)

// NewIssueID generates a new server-side ULID for an issue.
func NewIssueID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewOperationID generates a new server-side ULID for an operation.
func NewOperationID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewLifecycleFactID generates a new server-side ULID for a lifecycle fact.
func NewLifecycleFactID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewEnrichmentEventID generates a new server-side ULID for an enrichment event.
func NewEnrichmentEventID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewTagEventID generates a new server-side ULID for a tag event.
func NewTagEventID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewIssueContentFactID generates a new server-side ULID for an issue content fact.
func NewIssueContentFactID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewMemoryID generates a new server-side ULID for a memory.
func NewMemoryID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}

// NewMemoryProposalID generates a new server-side ULID for a memory proposal.
func NewMemoryProposalID() string {
	return ulid.MustNew(ulid.Now(), rand.Reader).String()
}
