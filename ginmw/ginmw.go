// Пакет ginmw предоставляет middleware Gin для HTTP-сервисов платформы CBL:
// идентификатор запроса, журнал доступа, recovery и ошибки в формате problem+json.
package ginmw

import (
	"errors"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/cbl/cbl-kit/grpcmw"
)

// Имена заголовков и типов содержимого, используемых платформой.
const (
	// HeaderRequestID задает имя заголовка идентификатора запроса.
	HeaderRequestID = "X-Request-Id"
	// ContentTypeProblem задает тип содержимого ошибок RFC 9457.
	ContentTypeProblem = "application/problem+json"
	requestIDKey       = "request_id"
)

// Problem описывает тело ошибки в формате application/problem+json (RFC 9457).
type Problem struct {
	// Type задает URI типа проблемы.
	Type string `json:"type"`
	// Title задает краткое описание проблемы.
	Title string `json:"title"`
	// Status задает HTTP-статус.
	Status int `json:"status"`
	// Code задает машиночитаемый код, например idempotency_key_reuse.
	Code string `json:"code,omitempty"`
	// Detail задает детали для не-внутренних ошибок.
	Detail string `json:"detail,omitempty"`
	// RequestID задает идентификатор запроса.
	RequestID string `json:"request_id,omitempty"`
}

// RequestID возвращает middleware, обеспечивающий наличие идентификатора запроса.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader(HeaderRequestID))
		if id == "" {
			if v7, err := uuid.NewV7(); err == nil {
				id = v7.String()
			}
		}
		c.Set(requestIDKey, id)
		c.Header(HeaderRequestID, id)
		c.Next()
	}
}

// RequestIDFrom возвращает идентификатор запроса из контекста Gin.
func RequestIDFrom(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// AccessLog логирует метод, путь, статус и длительность запроса.
func AccessLog(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", status),
			zap.Duration("duration", time.Since(start)),
			zap.String("request_id", RequestIDFrom(c)),
		}
		switch {
		case status >= http.StatusInternalServerError:
			log.Error("http request failed", fields...)
		case status >= http.StatusBadRequest:
			log.Warn("http request rejected", fields...)
		default:
			log.Info("http request", fields...)
		}
	}
}

// Recovery перехватывает паники и отдает 500 в формате problem+json.
func Recovery(log *zap.Logger) gin.HandlerFunc {
	if log == nil {
		log = zap.NewNop()
	}
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("stack", string(debug.Stack())),
					zap.String("request_id", RequestIDFrom(c)),
				)
				if !c.Writer.Written() {
					AbortWithProblem(c, http.StatusInternalServerError, "internal_error", "Internal Server Error", "")
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}

// AbortWithProblem прерывает обработку и отдает ошибку в формате problem+json.
func AbortWithProblem(c *gin.Context, status int, code, title, detail string) {
	problem := Problem{
		Type:      "about:blank",
		Title:     title,
		Status:    status,
		Code:      code,
		Detail:    detail,
		RequestID: RequestIDFrom(c),
	}
	c.Header("Content-Type", ContentTypeProblem)
	c.AbortWithStatusJSON(status, problem)
}

// AbortFromError маппит доменную ошибку в HTTP-статус и problem+json.
// Детали внутренних ошибок наружу не отдаются.
func AbortFromError(c *gin.Context, err error) {
	kind := grpcmw.KindOf(err)
	status, code, title := httpStatusFor(kind)
	detail := ""
	if kind != grpcmw.KindInternal {
		if domainCode := grpcmw.CodeOf(err); domainCode != "" {
			code = domainCode
		}
		detail = errorMessage(err)
	}
	AbortWithProblem(c, status, code, title, detail)
}

// httpStatusFor переводит класс ошибки в HTTP-статус, код и заголовок.
func httpStatusFor(kind grpcmw.Kind) (int, string, string) {
	switch kind {
	case grpcmw.KindNotFound:
		return http.StatusNotFound, "not_found", "Not Found"
	case grpcmw.KindInvalidArgument:
		return http.StatusUnprocessableEntity, "unprocessable_entity", "Unprocessable Entity"
	case grpcmw.KindAlreadyExists:
		return http.StatusConflict, "already_exists", "Conflict"
	case grpcmw.KindFailedPrecondition:
		return http.StatusConflict, "failed_precondition", "Conflict"
	case grpcmw.KindResourceExhausted:
		return http.StatusTooManyRequests, "resource_exhausted", "Too Many Requests"
	case grpcmw.KindUnavailable:
		return http.StatusServiceUnavailable, "unavailable", "Service Unavailable"
	default:
		return http.StatusInternalServerError, "internal_error", "Internal Server Error"
	}
}

// errorMessage извлекает сообщение доменной ошибки.
func errorMessage(err error) string {
	var target *grpcmw.Error
	if errors.As(err, &target) {
		return target.Message
	}
	return ""
}
