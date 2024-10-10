FROM golang:1.23-alpine AS builder

RUN apk --update --no-cache add upx
RUN addgroup --gid 10001 appgroup && \
    adduser --uid 10001 --ingroup appgroup --shell /sbin/nologin --disabled-password --no-create-home appuser

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go install -ldflags="-w -s" ./...
RUN upx /go/bin/scheduler

FROM scratch
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /go/bin/scheduler /app/scheduler

WORKDIR /app
USER appuser:appgroup
ENTRYPOINT ["/app/scheduler"]