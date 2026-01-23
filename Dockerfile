# Feedback Service
# Built by Blue Fermion Labs - https://bluefermionlabs.com

FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /feedback ./cmd/server

# Runtime image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary
COPY --from=builder /feedback /app/feedback

# Copy static files
COPY widget/ /app/widget/

# Create data directory
RUN mkdir -p /data

ENV PORT=8080
ENV FEEDBACK_DB_PATH=/data/feedback.db

EXPOSE 8080

ENTRYPOINT ["/app/feedback"]
