package middleware

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/newrelic/go-agent/v3/newrelic"

	"github.com/Naitik2411/go-tasker/internal/server"
)

type TracingMiddleware struct {
	server *server.Server
	nrApp  *newrelic.Application
}

func NewTracingMiddleware(s *server.Server, nrApp *newrelic.Application) *TracingMiddleware {
	return &TracingMiddleware{
		server: s,
		nrApp:  nrApp,
	}
}

// NewRelicMiddleware returns the New Relic middleware for Echo
func (tm *TracingMiddleware) NewRelicMiddleware() echo.MiddlewareFunc {
	if tm.nrApp == nil {
		return func(next echo.HandlerFunc) echo.HandlerFunc {
			return next
		}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			txn := tm.nrApp.StartTransaction(c.Request().URL.Path)
			defer txn.End()

			txn.SetWebRequestHTTP(c.Request())
			txn.SetWebResponse(c.Response())

			ctx := newrelic.NewContext(c.Request().Context(), txn)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// EnhanceTracing adds custom attributes to New Relic transactions
func (tm *TracingMiddleware) EnhanceTracing() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			txn := newrelic.FromContext(c.Request().Context())
			if txn == nil {
				return next(c)
			}

			txn.AddAttribute("http.real_ip", c.RealIP())
			txn.AddAttribute("http.user_agent", c.Request().UserAgent())

			if requestID := GetRequestID(c); requestID != "" {
				txn.AddAttribute("request.id", requestID)
			}

			if userID := c.Get("user_id"); userID != nil {
				if userIDStr, ok := userID.(string); ok {
					txn.AddAttribute("user.id", userIDStr)
				}
			}

			rw := &responseWriter{ResponseWriter: c.Response(), status: http.StatusOK}
			c.SetResponse(rw)

			err := next(c)
			if err != nil {
				txn.NoticeError(err)
			}

			txn.AddAttribute("http.status_code", rw.status)

			return err
		}
	}
}
