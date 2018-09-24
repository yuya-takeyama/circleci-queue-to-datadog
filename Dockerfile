FROM golang:1.11-alpine AS builder

ADD . /go/src/github.com/yuya-takeyama/circleci-queue-to-datadog
WORKDIR /go/src/github.com/yuya-takeyama/circleci-queue-to-datadog

RUN apk --update add git perl && \
  COMMIT_HASH=`git describe --always | perl -pe chomp` && \
  go build -ldflags "-X main.gitCommit=${COMMIT_HASH}"

FROM alpine:3.8

RUN apk --update add ca-certificates
COPY --from=builder /go/src/github.com/yuya-takeyama/circleci-queue-to-datadog/circleci-queue-to-datadog /circleci-queue-to-datadog

ENTRYPOINT ["/circleci-queue-to-datadog"]
