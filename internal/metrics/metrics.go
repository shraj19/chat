// package metrics define Prometheus metrics for relay.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// HTTP layer
var (
	// HTTPRequestsTotal count all HTTP requests, labeled by method, path and status_code.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "http_total_requests",
			Help:      "Total number of HTTP requests",
		},
		[]string{"method", "path", "status_code"},
	)

	// HTTPRequestsDuration measures the duration of HTTP requests, labeled by method and path.
	HTTPRequestsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "relay",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency distributions in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
		},
		[]string{"method", "path"},
	)
)

// Websocket layer
var (
	// WSConnectionsActive measures the number of active WebSocket connections.
	WSConnectionsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "relay",
			Name:      "ws_connections_active",
			Help:      "Number of active WebSocket connections",
		},
	)

	// WSMessagesTotal counts the total number of WebSocket messages, labeled by direction.
	WSMessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "ws_messages_total",
			Help:      "Total websocket messages sent and received",
		},
		[]string{"direction"}, // inbound or outbound
	)
)

// Cache layer

var (
	// CacheHitsTotal counts the total number of cache hits, labeled by cache type.
	CacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "cache_hits_total",
			Help:      "Total cache hits",
		},
		[]string{"cache"}, // message, IsParticipant etc.
	)

	// CacheMissesTotal counts the total number of cache misses, labeled by cache type
	CacheMissesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "cache_misses_total",
			Help:      "Total cache misses",
		},
		[]string{"cache"},
	)

	// CacheOperationsDuration observes cache operation latency by cache name and operation type.
	CacheOperationsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "relay",
			Name:      "cache_operations_duration_seconds",
			Help:      "Cache operation latency distributions in seconds",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~3s
		},
		[]string{"cache", "operation"}, // get, set, invalidation etc.
	)

	// CacheErrorsTotal counts cache backend errors that forced a fallback to the
	// source of truth (DB). Distinct from a miss: a miss means the cache was
	// healthy but empty; an error means the cache was unreachable/failed.
	CacheErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "cache_errors_total",
			Help:      "Cache backend errors that forced a fallback to the database",
		},
		[]string{"cache", "operation"},
	)
)

// Database layer

var (
	// DBQueryDuration measures the duration of database queries, labeled by operation (e.g., get_messages, create_message).
	DBQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "relay",
			Name:      "db_query_duration_seconds",
			Help:      "Database query latency",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.5, 1},
		},
		[]string{"operation"}, // "get_messages", "create_message", "list_conversations", etc.
	)

	// DBQueriesTotal counts the total number of database queries, labeled by operation and status (success or error).
	DBQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "db_queries_total",
			Help:      "Total database queries",
		},
		[]string{"operation", "status"}, // status: "success", "error"
	)
)

// Rate Limiter layer

var (
	// RateLimitHitsTotal counts the total number of requests rejected by the rate limiter, labeled by limiter type (e.g., login, signup, api, ws).
	RateLimitHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "relay",
			Name:      "rate_limit_hits_total",
			Help:      "Total requests rejected by rate limiter",
		},
		[]string{"limiter"}, // "login", "signup", "api", "ws"
	)
)

// Registration

// init registers all the metrics with Prometheus. This is called automatically when the package is imported.

// func init() {
// 	prometheus.MustRegister(
// 		HttpRequestsTotal,
// 		HttpRequestDuration,
// 		WSConnectionsActive,
// 		WSMessagesTotal,
// 		CacheHitsTotal,
// 		CacheMissesTotal,
// 		CacheOperationsDuration,
// 		DBQueryDuration,
// 		DBQueriesTotal,
// 		RateLimitHitsTotal,
// 	)
// }

// Register registers all the metrics with the provided Prometheus registerer.
//
// In Production, pass prometheus.DefaultRegisterer. In tests, pass a
// fresh prometheus.NewRegistry() for isolated metric collection.
//
// Usage:
//
// reg := promethus.NewRegistry()
// metrics.Register(reg)

func Register(reg prometheus.Registerer) {
	reg.MustRegister(
		HTTPRequestsTotal,
		HTTPRequestsDuration,
		WSConnectionsActive,
		WSMessagesTotal,
		CacheHitsTotal,
		CacheMissesTotal,
		CacheOperationsDuration,
		CacheErrorsTotal,
		DBQueryDuration,
		DBQueriesTotal,
		RateLimitHitsTotal,
	)
}
