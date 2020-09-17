# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.
FROM golang:1.12 as build-env
MAINTAINER Amazon Web Services, Inc.

WORKDIR /go/src/sleep
ADD ./sleep /go/src/sleep
RUN go get -d -v ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -installsuffix cgo -a -o /go/bin/sleep .

FROM scratch
MAINTAINER Amazon Web Services, Inc.
COPY --from=build-env /go/bin/sleep /
