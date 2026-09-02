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
COPY --from=studio /build/cmd/restitch-studio/dist/ ./cmd/restitch-studio/dist/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /restitch ./cmd/restitch
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /restitch-studio ./cmd/restitch-studio
# The docker-compose quickstart runs the mock upstream in the same image
# (finding H12); the image had no /mockupstream, so the stack was dead on
# arrival.
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /mockupstream ./cmd/mockupstream
# Studio database directory for the runtime image (finding H7). Owned by the
# distroless nonroot user (65532) so a fresh named volume mounted at /data
# inherits writable ownership.
RUN mkdir -p /data && chown 65532:65532 /data

# Stage 3: Runtime
FROM gcr.io/distroless/static-debian12
COPY --from=builder /restitch /restitch
COPY --from=builder /restitch-studio /restitch-studio
COPY --from=builder /mockupstream /mockupstream
COPY --from=builder /data /data
# Run as the distroless non-root user (finding H7).
USER nonroot
EXPOSE 8080 8443 9090 3080
ENTRYPOINT ["/restitch"]
