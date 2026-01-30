# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy root go.work and go.work.sum (if exists)
COPY go.work .
COPY go.work.sum* .

# Copy backend module
COPY backend/go.mod backend/go.sum* ./backend/
COPY backend/ ./backend/

# Download dependencies
WORKDIR /app/backend
RUN go mod download

# Build optimized binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server main.go

# Run Stage
FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/backend/server .
COPY --from=builder /app/backend/migrations ./migrations

EXPOSE 8080

CMD ["./server"]