# syntax=docker/dockerfile:1
# Multi-stage build for GoPool daemon and API
FROM node:20 AS web

WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.25 AS builder

WORKDIR /src

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .
COPY --from=web /src/internal/api/webdist ./internal/api/webdist

# Install build dependencies for cgo (sqlite)
RUN apt-get update && apt-get install -y --no-install-recommends gcc libc6-dev && rm -rf /var/lib/apt/lists/*

# Build daemon
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o gopool ./cmd

# Build API server (embeds frontend)
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o gopool-api ./cmd/api

# Runtime image
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /root

COPY --from=builder /src/gopool /src/gopool-api ./
COPY --from=builder /src/schema ./schema

# Expose ports for API and metrics
EXPOSE 8080 9100

# Choose service via env var
ENV SERVICE=daemon
ENTRYPOINT ["sh", "-c", "if [ \"$SERVICE\" = \"api\" ]; then exec ./gopool-api; else exec ./gopool; fi"]
