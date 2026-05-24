package issues

// RevisionBus is the seam between revision producers (mutation sites that call
// Bump) and consumers (HTTP handlers and long-lived subscribers such as the
// SSE /revision/stream endpoint).
//
// Today the only implementation is *RevisionTracker, which is in-process and
// single-node. A future multi-instance deployment can provide an alternative
// implementation (Redis pub/sub, NATS, etc.) that satisfies this interface;
// the HTTP handler and all Bump call sites remain unchanged.
type RevisionBus interface {
	// Revision returns the current revision number.
	Revision() uint64

	// Bump atomically advances the revision and notifies all subscribers.
	// Returns the new revision.
	Bump() uint64

	// Subscribe registers a subscriber for revision updates. The returned
	// channel is buffered with capacity 1 and uses drop-latest semantics:
	// if a subscriber hasn't drained the previous value, the publisher
	// overwrites it rather than blocking. Subscribers always converge to
	// the latest revision.
	//
	// Callers MUST invoke cancel to release resources; channel is closed
	// by cancel.
	Subscribe() (ch <-chan uint64, cancel func())
}

// Compile-time assertion that *RevisionTracker satisfies RevisionBus.
var _ RevisionBus = (*RevisionTracker)(nil)
