package inbox

import (
	"strconv"
	"strings"
)

// Retry and DLQ topic suffixes (CBL-003, section 6).
const (
	// RetrySuffix5s sets the suffix of the first retry attempt.
	RetrySuffix5s = ".retry.5s"
	// RetrySuffix1m sets the suffix of the second retry attempt.
	RetrySuffix1m = ".retry.1m"
	// RetrySuffix10m sets the suffix of the third retry attempt.
	RetrySuffix10m = ".retry.10m"
	// DLQSuffix sets the suffix of the dead letter queue.
	DLQSuffix = ".dlq"
)

// Retry headers.
const (
	// HeaderRetryAttempt sets the number of the failed processing attempt.
	HeaderRetryAttempt = "x-retry-attempt"
	// HeaderError sets the text of the last processing error.
	HeaderError = "x-error"
	// HeaderOriginalEventID preserves the original event_id during DLQ sanitization.
	HeaderOriginalEventID = "x-original-event-id"
)

// Router routes failed messages to retry topics and the DLQ.
type Router struct {
	mainTopic   string
	maxAttempts int
}

// NewRouter creates a Router; maxAttempts caps the number of retry attempts.
func NewRouter(mainTopic string, maxAttempts int) *Router {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Router{mainTopic: strings.TrimSpace(mainTopic), maxAttempts: maxAttempts}
}

// MainTopicFrom returns the main topic for the current delivery topic.
func MainTopicFrom(topic string) string {
	for _, suffix := range []string{RetrySuffix5s, RetrySuffix1m, RetrySuffix10m, DLQSuffix} {
		if strings.HasSuffix(topic, suffix) {
			return strings.TrimSuffix(topic, suffix)
		}
	}
	return topic
}

// Next returns the topic of the next attempt; isDLQ=true means delivery to the DLQ.
// attempt is the number of failed attempts after the current failure.
func (r *Router) Next(currentTopic string, attempt int) (string, bool) {
	base := MainTopicFrom(currentTopic)
	if base == "" {
		base = r.mainTopic
	}
	if attempt > r.maxAttempts {
		return base + DLQSuffix, true
	}
	switch attempt {
	case 1:
		return base + RetrySuffix5s, false
	case 2:
		return base + RetrySuffix1m, false
	default:
		return base + RetrySuffix10m, false
	}
}

// ParseAttempt parses the x-retry-attempt header; an invalid value is treated as zero.
func ParseAttempt(value string) int {
	attempt, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || attempt < 0 {
		return 0
	}
	return attempt
}
