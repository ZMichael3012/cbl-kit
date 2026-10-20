package ginmw_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ZMichael3012/cbl-kit/ginmw"
	"github.com/ZMichael3012/cbl-kit/grpcmw"
)

func newTestRouter(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ginmw.RequestID())
	engine.Use(ginmw.Recovery(zap.NewNop()))
	for _, h := range handlers {
		engine.Use(h)
	}
	return engine
}

func TestRequestIDGeneratedAndPropagated(t *testing.T) {
	t.Parallel()

	engine := newTestRouter()
	var fromContext string
	engine.GET("/test", func(c *gin.Context) {
		fromContext = ginmw.RequestIDFrom(c)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	header := rec.Header().Get(ginmw.HeaderRequestID)
	assert.NotEmpty(t, header)
	assert.Equal(t, header, fromContext)
}

func TestRequestIDFromHeader(t *testing.T) {
	t.Parallel()

	engine := newTestRouter()
	engine.GET("/test", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(ginmw.HeaderRequestID, "req-123")
	engine.ServeHTTP(rec, req)

	assert.Equal(t, "req-123", rec.Header().Get(ginmw.HeaderRequestID))
}

func TestAbortWithProblem(t *testing.T) {
	t.Parallel()

	engine := newTestRouter()
	engine.GET("/problem", func(c *gin.Context) {
		ginmw.AbortWithProblem(c, http.StatusConflict, "idempotency_key_reuse", "Conflict", "тело отличается")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/problem", nil)
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	assert.Equal(t, ginmw.ContentTypeProblem, rec.Header().Get("Content-Type"))

	var problem ginmw.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, http.StatusConflict, problem.Status)
	assert.Equal(t, "idempotency_key_reuse", problem.Code)
	assert.Equal(t, "тело отличается", problem.Detail)
	assert.NotEmpty(t, problem.RequestID)
}

func TestAbortFromError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not found", err: grpcmw.E(grpcmw.KindNotFound, "payment_not_found", "платеж не найден", nil), wantStatus: http.StatusNotFound, wantCode: "payment_not_found"},
		{name: "invalid", err: grpcmw.E(grpcmw.KindInvalidArgument, "bad_amount", "сумма некорректна", nil), wantStatus: http.StatusUnprocessableEntity, wantCode: "bad_amount"},
		{name: "exists", err: grpcmw.E(grpcmw.KindAlreadyExists, "duplicate", "уже есть", nil), wantStatus: http.StatusConflict, wantCode: "duplicate"},
		{name: "precondition", err: grpcmw.E(grpcmw.KindFailedPrecondition, "bad_state", "нельзя отменить", nil), wantStatus: http.StatusConflict, wantCode: "bad_state"},
		{name: "exhausted", err: grpcmw.E(grpcmw.KindResourceExhausted, "rate_limited", "лимит", nil), wantStatus: http.StatusTooManyRequests, wantCode: "rate_limited"},
		{name: "unavailable", err: grpcmw.E(grpcmw.KindUnavailable, "gateway_down", "шлюз недоступен", nil), wantStatus: http.StatusServiceUnavailable, wantCode: "gateway_down"},
		{name: "unauthenticated", err: grpcmw.E(grpcmw.KindUnauthenticated, "missing_token", "token is missing", nil), wantStatus: http.StatusUnauthorized, wantCode: "missing_token"},
		{name: "permission denied", err: grpcmw.E(grpcmw.KindPermissionDenied, "no_rights", "role has no rights", nil), wantStatus: http.StatusForbidden, wantCode: "no_rights"},
		{name: "internal", err: errors.New("secret detail"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			engine := newTestRouter()
			engine.GET("/err", func(c *gin.Context) { ginmw.AbortFromError(c, tt.err) })

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/err", nil)
			engine.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			var problem ginmw.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.Equal(t, tt.wantCode, problem.Code)
			if tt.wantStatus == http.StatusInternalServerError {
				assert.NotContains(t, rec.Body.String(), "secret detail")
			}
		})
	}
}

func TestRecovery(t *testing.T) {
	t.Parallel()

	engine := newTestRouter()
	engine.GET("/panic", func(c *gin.Context) { panic("boom") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	var problem ginmw.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "internal_error", problem.Code)
}

func TestAccessLog(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ginmw.RequestID())
	engine.Use(ginmw.AccessLog(zap.NewNop()))
	engine.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/bad", func(c *gin.Context) { c.Status(http.StatusBadRequest) })

	for _, path := range []string{"/ok", "/bad"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		engine.ServeHTTP(rec, req)
	}
}
