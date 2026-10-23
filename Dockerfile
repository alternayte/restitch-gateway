# Stage 1: Build Studio frontend
FROM node:24-alpine AS studio
WORKDIR /build/studio
COPY studio/package.json studio/package-lock.json ./
RUN npm ci
COPY studio/ ./
RUN npm run build

# Stage 2: Build Go binaries
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=studio /build/internal/studio/dist/ ./internal/studio/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /restitch ./cmd/restitch
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /mockupstream ./cmd/mockupstream
# SQLite directory for a single-node Studio on local disk. The distroless
# nonroot user (65532) writes here. Docker initializes a fresh named volume
# from this directory but keeps the volume ROOT owned by root, so the
# directory itself must be world-writable.
RUN mkdir -p /data && touch /data/.keep && chmod 777 /data && chown -R 65532:65532 /data

# The mock upstream of the Compose demo. Build it with --target mockupstream;
# the release image does not contain it.
FROM gcr.io/distroless/static-debian12 AS mockupstream
COPY --from=builder /mockupstream /mockupstream
USER nonroot
EXPOSE 8081
ENTRYPOINT ["/mockupstream"]

# Runtime: one binary for every role. There is no default command; each
# container names its role (gateway, studio, dev, check, publish, version).
FROM gcr.io/distroless/static-debian12
COPY --from=builder /restitch /restitch
COPY --from=builder /data /data
USER nonroot
EXPOSE 8080 8443 9090 3080
ENTRYPOINT ["/restitch"]
