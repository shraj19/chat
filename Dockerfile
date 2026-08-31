# Stage 1: Build the binary
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the binary with optimisations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# Stage 2: Create a runtime image with Redis and Go app
FROM alpine:3.19

# Install ca-certificates and redis server
RUN apk --no-cache add ca-certificates redis

WORKDIR /root/

# Copy the binary from the builder stage
COPY --from=builder /app/server .

# Copy entrypoint script and set executable permissions
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

# Expose app port and Redis port
EXPOSE 8080 6379

# Launch both Redis and application via entrypoint script
ENTRYPOINT ["./entrypoint.sh"]
