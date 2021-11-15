ARG WORK_DIR="/home/src"
ARG BASE_IMAGE="golang:1.16"

FROM ${BASE_IMAGE} as builder

COPY . /home/src
WORKDIR ${WORK_DIR}

RUN go build -mod=vendor -o retest ./main.go

FROM scratch

COPY --from=builder ${WORK_DIR}/retest /usr/local/bin/retest

ENTRYPOINT [ "/usr/local/bin/retest" ]
