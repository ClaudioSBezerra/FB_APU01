# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy backend module
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy source code
COPY backend/ .
RUN go mod tidy

# Build optimized binary
RUN CGO_ENABLED=0 GOOS=linux go build -v -ldflags="-w -s" -o server main.go

# Run Stage
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./server"]