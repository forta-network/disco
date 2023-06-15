FROM golang:1.19-alpine3.18 AS build

ENV DISCO_DIR /go/src/github.com/forta-network/disco

WORKDIR ${DISCO_DIR}
COPY . ${DISCO_DIR}
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /disco/disco .

FROM alpine:3.18

RUN set -ex \
    && apk add --no-cache ca-certificates

COPY --from=build /disco /disco
ENV REGISTRY_CONFIGURATION_PATH /disco/config.yaml

EXPOSE 1970
ENTRYPOINT ["/disco/disco"]
