FROM golang:alpine@sha256:926f7f7e1ab8509b4e91d5ec6d5916ebb45155b0c8920291ba9f361d65385806

RUN apk --no-cache add make git curl bash fish

WORKDIR /project

COPY ./ /project
RUN make tools

CMD make
