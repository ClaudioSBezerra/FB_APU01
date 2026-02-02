# Build Stage
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app

# Copy go.mod and go.sum from the backend directory
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy the rest of the backend source code
COPY backend/ .

# Ensure dependencies are tidy
RUN go mod tidy

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-w -s" -o server .

# Run Stage
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080
CMD ["./server"]
# Force Rebuild v4.4.1