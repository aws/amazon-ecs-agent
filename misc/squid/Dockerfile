FROM alpine:3.6
RUN apk update && apk add squid
EXPOSE 3128
ENTRYPOINT ["squid", "-Nd", "1"]
