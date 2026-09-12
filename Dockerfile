ARG GO_VERSION=1.27.1

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION} AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    for dir in $(find ./cmd -mindepth 1 -maxdepth 1 -type d); do \
        binary_name=$(basename "$dir"); \
        echo "Building: $binary_name"; \
        CGO_ENABLED=0 \
        GOOS=${TARGETOS} \
        GOARCH=${TARGETARCH} \
        go build -v -buildvcs=true -trimpath -ldflags="-s -w" \
          -o /go/bin/"$binary_name" "$dir"; \
    done

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /go/bin/ /usr/bin/
USER 65532:65532
ENTRYPOINT ["/usr/bin/scheduler"]
