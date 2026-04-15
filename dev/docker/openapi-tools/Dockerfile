FROM node:24-alpine

RUN mkdir /data

RUN npm install -g openapi-format@1.29.3 @redocly/cli@2.19.0

COPY format /usr/bin/format

RUN chmod +x /usr/bin/format

WORKDIR /data
