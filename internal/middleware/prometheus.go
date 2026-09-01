package middleware

import (
	"net/http"
	"strconv"
	"time"

	"chat-v2/internal/metrics"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

/**
 *	WriteHeader overrides the default WriteHeader method to capture the HTTP status code for metrics.
 *	Parameters:
 *	- code: The HTTP status code to be written to the response.
 */
func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

/**
 *	Metrics is a middleware that collects Prometheus metrics for HTTP requests.
 *	It records the total number of requests and the duration of each request, excluding certain paths like /metrics and /health.
 *	Returns:
 *	- A function that takes an http.Handler and returns an http.Handler with metrics collection.
 */
func Metrics() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// path extraction and filteration for metrics
			if r.URL.Path == "/metrics" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			recorder := newStatusRecorder(w)
			next.ServeHTTP(recorder, r)

			duration := time.Since(start)
			status := strconv.Itoa(recorder.status)

			// Record metrics
			// Use r.Pattern to get the route pattern instead of r.URL.Path for better aggregation in Prometheus
			metrics.HTTPRequestsTotal.WithLabelValues(r.Method, r.Pattern, status).Inc()
			metrics.HTTPRequestsDuration.WithLabelValues(r.Method, r.Pattern).Observe(duration.Seconds())

		})
	}
}
