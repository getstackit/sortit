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
