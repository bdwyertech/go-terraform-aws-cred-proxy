FROM golang:1.14-alpine as helper
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 GOFLAGS=-mod=vendor go build .

FROM library/alpine:3.11
COPY --from=helper /build/terraform-aws-cred-helper /usr/local/bin/

RUN adduser credhelper -S -h /home/credhelper

USER credhelper
WORKDIR /home/credhelper

CMD "/usr/local/bin/terraform-aws-cred-helper"