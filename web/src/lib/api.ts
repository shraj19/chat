import { TanStackApiService } from "wire-axon/services";
import { CookieStrategy } from "wire-axon/auth";
import { QueryClient } from "@tanstack/react-query";

// API service with cookie auth (httpOnly access_token cookie)
export const apiService = new TanStackApiService({
  baseURL: "/api", // proxied by Vite to localhost:8080
  withCredentials: true,
  auth: new CookieStrategy({}),
  retry: {
    maxRetries: 2,
    baseDelay: 500,
    retryableStatuses: [500, 502, 503, 504],
  },
});

// Query client for TanStack Query
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000, // 5 minutes
      gcTime: 10 * 60 * 1000, // 10 minutes
      refetchOnWindowFocus: false,
      retry: 1,
    },
  },
});
