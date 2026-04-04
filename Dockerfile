# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o app main.go

# Run stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/app .

# Copy .env file if exists
COPY --from=builder /app/.env ./

# Copy public folder
COPY --from=builder /app/public ./public/

EXPOSE 3000

CMD ["./app"]
