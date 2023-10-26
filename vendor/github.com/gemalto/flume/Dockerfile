FROM golang:1.14-alpine@sha256:e196a762bd349b188d66c539354fc23f0e5a0178c214e6dfe4d7894c3c2753f1

RUN apk --no-cache add make bash fish build-base

WORKDIR /flume

COPY ./Makefile ./go.mod ./go.sum /flume/
RUN make tools

COPY ./ /flume

CMD make all
