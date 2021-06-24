# escape=`

FROM golang:1.12 as build-env
MAINTAINER Amazon Web Services, Inc.

# There is dockerfile documentation on how to treat windows paths
WORKDIR C:\Users\Administrator\go\src\sleep
COPY ./sleep C:/Users/Administrator/go/src/sleep
RUN go build -tags integration -installsuffix cgo -a -o C:/Users/Administrator/go/src/sleep/sleep.exe .

FROM amazon-ecs-ftest-windows-base:make
MAINTAINER Amazon Web Services, Inc.
COPY --from=build-env C:/Users/Administrator/go/src/sleep/sleep.exe C:/
