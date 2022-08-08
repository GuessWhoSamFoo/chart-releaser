# syntax=docker/dockerfile:1
FROM golang:stretch AS builder

WORKDIR /go/src/chart-releaser

COPY . .

RUN make build

FROM alpine/git:v2.36.2

LABEL org.label-schema.schema-version="1.0" \
      org.label-schema.name="chartreleaser/chart-releaser" \
      org.label-schema.vcs-url="https://github.com/vapor-ware/chart-releaser" \
      org.label-schema.vendor="Vapor IO"
COPY --from=builder /go/src/chart-releaser/chart-releaser /usr/local/bin/chart-releaser

ENV WORKDIR /data
WORKDIR /data

ENTRYPOINT ["chart-releaser"]
