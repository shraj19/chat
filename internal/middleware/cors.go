package middleware

import (
	"net/http"

	"github.com/rs/cors"
)

/**
 *	CORS is a middleware that handles Cross-Origin Resource Sharing (CORS) for HTTP requests.
 *	It allows you to specify which origins are permitted to access the server's resources.
 *	Parameters:
 *	- allowedOrigins: A list of origins that are allowed to make cross-origin requests.
 */
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	c := cors.New(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowedHeaders: []string{
			"Authorization",
			"Content-Type",
			"Accept",
			"Origin",
			"X-Requested-With",
		},
		AllowCredentials: true,
		MaxAge:           300,
	})

	return c.Handler
}
