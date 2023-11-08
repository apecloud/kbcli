# Build the kbcli binaries

## docker buildx build injected build-args:
#BUILDPLATFORM — matches the current machine. (e.g. linux/amd64)
#BUILDOS — os component of BUILDPLATFORM, e.g. linux
#BUILDARCH — e.g. amd64, arm64, riscv64
#BUILDVARIANT — used to set build ARM variant, e.g. v7
#TARGETPLATFORM — The value set with --platform flag on build
#TARGETOS - OS component from --platform, e.g. linux
#TARGETARCH - Architecture from --platform, e.g. arm64
#TARGETVARIANT - used to set target ARM variant, e.g. v7

ARG GO_VERSION=1.21

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} as builder
ARG TARGETOS
ARG TARGETARCH
ARG GOPROXY
#ARG GOPROXY=https://goproxy.cn
ARG LD_FLAGS="-s -w"

ENV GONOPROXY=github.com/apecloud
ENV GONOSUMDB=github.com/apecloud
ENV GOPRIVATE=github.com/apecloud
ENV GOPROXY=${GOPROXY}

WORKDIR /src

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
# RUN go mod download
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Build
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="${LD_FLAGS}" -tags="containers_image_openpgp" -a -o /out/kbcli cmd/cli/main.go

# Use alpine with tag 20230329 is corresponding to "edge" tag (latest release to date is 3.18) as of 20230625
FROM docker.io/alpine:edge as dist
ARG APK_MIRROR

# install tools via apk
ENV APK_MIRROR=${APK_MIRROR}
RUN if [ -n "${APK_MIRROR}" ]; then sed -i "s/dl-cdn.alpinelinux.org/${APK_MIRROR}/g" /etc/apk/repositories; fi
RUN apk add --no-cache curl kubectl helm jq --allow-untrusted \
    && rm -rf /var/cache/apk/*

# copy kbcli
COPY --from=builder /out/kbcli /bin

# mkdir kbcli config dir and helm cache dir.
RUN mkdir /.kbcli && chown -R 65532:65532 /.kbcli \
    && mkdir /.cache && chown -R 65532:65532 /.cache
USER 65532:65532

