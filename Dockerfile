ARG BASE_IMAGE=gcr.io/distroless/static:nonroot

FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BUILD_HASH=dev
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${BUILD_HASH}" \
    -o /out/cache-server ./cmd/cache-server

FROM ${BASE_IMAGE}
WORKDIR /
COPY --from=builder /out/cache-server /cache-server
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/cache-server"]
