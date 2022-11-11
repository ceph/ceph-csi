ARG WORK_DIR="/home/src"
ARG BASE_IMAGE="golang:1.18"

FROM ${BASE_IMAGE} as builder

ARG WORK_DIR

COPY . /home/src
WORKDIR ${WORK_DIR}

RUN go build -mod=vendor -o retest ./main.go

FROM ${BASE_IMAGE}

ARG WORK_DIR

COPY --from=builder ${WORK_DIR}/retest /usr/local/bin/retest

ENTRYPOINT [ "/usr/local/bin/retest" ]
