FROM golang:1.19.1-alpine as dev
EXPOSE 8080
ENV CGO_ENABLED 0
WORKDIR /go/src/app
RUN apk update && apk add git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go install github.com/cosmtrek/air@latest
CMD ["air", "-c", ".air.toml"]

FROM golang:1.19.1-alpine as builder
WORKDIR /go/src/app
RUN apk update && apk add git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o ./main ./src/

FROM scratch as prod
EXPOSE 8080
WORKDIR /go/src/app
COPY --from=builder /go/src/app/main .
COPY --from=builder /go/src/app/.kube/config ./.kube/
CMD ["/go/src/app/main"]