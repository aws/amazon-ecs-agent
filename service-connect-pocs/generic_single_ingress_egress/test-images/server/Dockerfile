FROM public.ecr.aws/bitnami/golang:latest AS build
WORKDIR /go/src/server/
COPY server.go ./
COPY go.mod ./
RUN CGO_ENABLED=0 GOOS=linux GOPROXY=direct go build -a -installsuffix cgo -o server .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
RUN apk --no-cache add curl
WORKDIR /root/
COPY --from=build /go/src/server/server ./
CMD ["./server"]
