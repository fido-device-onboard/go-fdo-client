# SPDX-License-Identifier: Apache 2.0

FROM golang:1.25-alpine AS builder

WORKDIR /go/src/app
COPY . .

RUN apk add make
RUN make
RUN install -D -m 755 go-fdo-client /go/bin/

# Start a new stage
FROM gcr.io/distroless/static-debian12

COPY --from=builder /go/bin/go-fdo-client /usr/bin/go-fdo-client

ENTRYPOINT ["go-fdo-client"]
