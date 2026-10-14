package inbox

import (
	"strconv"
	"strings"
)

// Суффиксы топиков повторной доставки и DLQ (CBL-003, раздел 6).
const (
	// RetrySuffix5s задает суффикс первой повторной попытки.
	RetrySuffix5s = ".retry.5s"
	// RetrySuffix1m задает суффикс второй повторной попытки.
	RetrySuffix1m = ".retry.1m"
	// RetrySuffix10m задает суффикс третьей повторной попытки.
	RetrySuffix10m = ".retry.10m"
	// DLQSuffix задает суффикс очереди необработанных сообщений.
	DLQSuffix = ".dlq"
)

// Заголовки повторной доставки.
const (
	// HeaderRetryAttempt задает номер неудачной попытки обработки.
	HeaderRetryAttempt = "x-retry-attempt"
	// HeaderError задает текст последней ошибки обработки.
	HeaderError = "x-error"
	// HeaderOriginalEventID сохраняет исходный event_id при санитизации DLQ.
	HeaderOriginalEventID = "x-original-event-id"
)

// Router маршрутизирует неуспешные сообщения в retry-топики и DLQ.
type Router struct {
	mainTopic   string
	maxAttempts int
}

// NewRouter создает Router; maxAttempts ограничивает число повторных попыток.
func NewRouter(mainTopic string, maxAttempts int) *Router {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &Router{mainTopic: strings.TrimSpace(mainTopic), maxAttempts: maxAttempts}
}

// MainTopicFrom возвращает основной топик по текущему топику доставки.
func MainTopicFrom(topic string) string {
	for _, suffix := range []string{RetrySuffix5s, RetrySuffix1m, RetrySuffix10m, DLQSuffix} {
		if strings.HasSuffix(topic, suffix) {
			return strings.TrimSuffix(topic, suffix)
		}
	}
	return topic
}

// Next возвращает топик следующей попытки; isDLQ=true означает отправку в DLQ.
// attempt равен числу неудачных попыток после текущего сбоя.
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

// ParseAttempt разбирает заголовок x-retry-attempt; некорректное значение считается нулем.
func ParseAttempt(value string) int {
	attempt, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || attempt < 0 {
		return 0
	}
	return attempt
}
