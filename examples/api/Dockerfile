# Build stage
FROM golang:1.25.4-alpine3.22 AS builder

# Set up directory structure matching the replace directive (../..)
WORKDIR /workspace/examples/api

# Copy the parent goai module to /workspace (two levels up from /workspace/examples/api)
COPY go.mod go.sum /workspace/
COPY agent /workspace/agent
COPY message /workspace/message
COPY provider /workspace/provider
COPY schema /workspace/schema
COPY tool /workspace/tool

# Copy example's go mod files
COPY examples/api/go.mod examples/api/go.sum ./
RUN go mod download

# Copy example source code
COPY examples/api/ .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server/main.go

# Production stage
FROM alpine:3.22

WORKDIR /app

# Copy binary from builder
COPY --from=builder /workspace/examples/api/server .

# Expose port
EXPOSE 8080

# Run server
CMD ["./server"]
