package tarodomain

import "strings"

// StreamAll is the ID of the global stream: every event of every aggregate
// (§3.1).
const StreamAll = "$all"

// uuidTextLen is the length of a hyphenated UUID.
const uuidTextLen = 36

// StreamKind tells what a stream ID names.
type StreamKind int

const (
	// StreamKindCategory is a stream of every event of one aggregate type,
	// such as "BankAccount".
	StreamKindCategory StreamKind = iota
	// StreamKindAggregate is the stream of one aggregate, such as
	// "BankAccount-01909e3c-...".
	StreamKindAggregate
	// StreamKindAll is the global stream, StreamAll.
	StreamKindAll
)

// String returns "category", "aggregate" or "all".
func (k StreamKind) String() string {
	switch k {
	case StreamKindAggregate:
		return "aggregate"
	case StreamKindAll:
		return "all"
	default:
		return "category"
	}
}

// StreamForAggregate returns the stream ID of one aggregate: its type, a
// hyphen and its lower-case UUID. The type may itself contain hyphens.
func StreamForAggregate(aggregateType string, id AggregateID) string {
	return aggregateType + "-" + id.String()
}

// StreamRef is a parsed stream ID.
type StreamRef struct {
	// Kind says which of the three forms the ID has.
	Kind StreamKind
	// Category is the aggregate type. It is empty for StreamAll.
	Category string
	// AggregateID is set only for StreamKindAggregate.
	AggregateID AggregateID
}

// ParseStreamID splits a stream ID into its kind, category and aggregate ID.
//
// An aggregate stream ID ends in a hyphen and a lower-case hyphenated UUID, so
// the type is everything before the last 37 characters and may contain
// hyphens. Anything else, including an ID whose tail is an upper-case or
// non-canonical UUID, is a category.
func ParseStreamID(streamID string) StreamRef {
	if streamID == StreamAll {
		return StreamRef{Kind: StreamKindAll}
	}
	n := len(streamID)
	if n > uuidTextLen+1 && streamID[n-uuidTextLen-1] == '-' {
		tail := streamID[n-uuidTextLen:]
		if tail == strings.ToLower(tail) {
			if u, err := ParseUUID(tail); err == nil {
				return StreamRef{
					Kind:        StreamKindAggregate,
					Category:    streamID[:n-uuidTextLen-1],
					AggregateID: AggregateID{u},
				}
			}
		}
	}
	return StreamRef{Kind: StreamKindCategory, Category: streamID}
}

// CategoryOf returns the category a stream belongs to: the aggregate type for
// an aggregate or category stream, and "" for StreamAll.
func CategoryOf(streamID string) string { return ParseStreamID(streamID).Category }
