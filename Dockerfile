FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata 

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application with security flags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -extldflags '-static'" -o main .

# Use a minimal image for the runtime
FROM alpine:latest

# Add non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Set timezone
ENV TZ=UTC

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/main .

# Copy any necessary config files
COPY --from=builder /app/.env .env.example

# Create directory for persistent data with correct permissions
RUN mkdir -p /app/audio_samples && \
    chown -R appuser:appgroup /app

# Use the non-root user
USER appuser

# Expose port
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/health || exit 1

# Run the binary
CMD ["./main"] 