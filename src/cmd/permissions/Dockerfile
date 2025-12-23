# syntax=docker/dockerfile:1.6
ARG GO_VERSION=1.22
FROM golang:${GO_VERSION} AS builder
WORKDIR /workspace
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN --mount=type=cache,target=/go/pkg/mod \
  CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -o /out/permissions ./cmd/permissions

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=builder /out/permissions /permissions
USER nonroot:nonroot
ENTRYPOINT ["/permissions"]
