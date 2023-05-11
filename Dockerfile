FROM golang:1.20-alpine AS builder
ARG VERSION
ARG GOPROXY
RUN apk --update add upx
RUN adduser -D -g '' appuser
WORKDIR /src
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY cmd cmd
COPY pkg pkg
RUN env CGO_ENABLED=0 go install -ldflags="-w -s -X main.version=${VERSION}" ./...
RUN upx /go/bin/scheduler

FROM scratch
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /go/bin/scheduler /usr/bin/scheduler
USER appuser
ENTRYPOINT ["/usr/bin/scheduler"]