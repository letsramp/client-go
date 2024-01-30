FROM  golang:1.19.2-alpine3.15 AS build
RUN apk --no-cache add curl
COPY ../../echo /app
WORKDIR /app
RUN go mod tidy
RUN go build -o server ./server
RUN go build -o client ./client

ENTRYPOINT ["/app/server/server"]
