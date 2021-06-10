FROM golang:1.16-alpine3.13 AS build

ENV DISCO_DIR /go/src/github.com/OpenZeppelin/disco

WORKDIR ${DISCO_DIR}
COPY . ${DISCO_DIR}
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /disco/disco .
COPY ./config/default-config.yaml /disco/config.yaml

FROM alpine:3.12

RUN set -ex \
    && apk add --no-cache ca-certificates

COPY --from=build /disco /disco
ENV REGISTRY_CONFIGURATION_PATH /disco/config.yaml

EXPOSE 5000
ENTRYPOINT ["/disco/disco"]
