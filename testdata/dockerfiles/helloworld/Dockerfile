FROM golang:1.19.2-alpine3.15 AS build
COPY ../../helloworld /app
WORKDIR /app
RUN go mod tidy
RUN go build -o server ./server/main.go
RUN go build -o client ./client/main.go

ENTRYPOINT ["/app/server/main"]
