FROM alpine:3.20

RUN apk add --no-cache jq

RUN mkdir /data
RUN mkdir /jq
COPY *.jq /jq
RUN chmod +x /jq/*.jq

WORKDIR /data
