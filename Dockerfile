FROM golang:1.12 AS dev
WORKDIR /go/src/github.com/djmaze/docker-plugin-volume-mounter
CMD ["go", "build"]

FROM dev AS builder
COPY go/src /go/src
COPY src ./
RUN CGO_ENABLED=0 GOOS=linux go get -tags netgo -ldflags "-s -w"

FROM alpine AS prod
COPY start-container /usr/local/bin/
COPY --from=builder /go/src/github.com/djmaze/docker-plugin-volume-mounter /usr/local/bin/

CMD ["docker-plugin-volume-mounter"]
