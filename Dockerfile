FROM golang:1.12 AS dev
WORKDIR /usr/src/app
CMD ["go", "build"]

FROM dev AS builder
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -tags netgo -ldflags "-s -w"

FROM alpine AS prod
COPY start-container /usr/local/bin/
COPY --from=builder /usr/src/app/docker-plugin-volume-mounter /usr/local/bin/

CMD ["docker-plugin-volume-mounter"]
