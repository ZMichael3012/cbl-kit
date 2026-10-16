// Package ginmw provides Gin middleware for CBL platform HTTP services:
// request ID, access log, recovery, and errors in the problem+json format.
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

// Names of headers and content types used by the platform.
const (
	// HeaderRequestID sets the request identifier header name.
	HeaderRequestID = "X-Request-Id"
	// ContentTypeProblem sets the content type of RFC 9457 errors.
	ContentTypeProblem = "application/problem+json"
	requestIDKey       = "request_id"
)

// Problem describes an error body in the application/problem+json format (RFC 9457).
type Problem struct {
	// Type sets the problem type URI.
	Type string `json:"type"`
	// Title sets a short problem description.
	Title string `json:"title"`
	// Status sets the HTTP status.
	Status int `json:"status"`
	// Code sets a machine-readable code, for example idempotency_key_reuse.
	Code string `json:"code,omitempty"`
	// Detail sets details for non-internal errors.
	Detail string `json:"detail,omitempty"`
	// RequestID sets the request identifier.
	RequestID string `json:"request_id,omitempty"`
}

// RequestID returns middleware that ensures a request identifier is present.
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

// RequestIDFrom returns the request identifier from the Gin context.
func RequestIDFrom(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// AccessLog logs the request method, path, status, and duration.
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

// Recovery catches panics and responds with 500 in the problem+json format.
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

// AbortWithProblem aborts processing and responds with an error in the problem+json format.
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

// AbortFromError maps a domain error to an HTTP status and problem+json.
// Details of internal errors are not exposed to the caller.
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

// httpStatusFor converts an error kind into an HTTP status, code, and title.
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

// errorMessage extracts the domain error message.
func errorMessage(err error) string {
	var target *grpcmw.Error
	if errors.As(err, &target) {
		return target.Message
	}
	return ""
}
