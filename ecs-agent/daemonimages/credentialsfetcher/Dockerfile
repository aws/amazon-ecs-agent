# Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
#       http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

# use minimal amazonlinux:2023 base for credentials-fetcher image
FROM public.ecr.aws/amazonlinux/amazonlinux:2023-minimal

RUN dnf install -y krb5-workstation jq unzip && mkdir /krb5 && chmod 755 /krb5
RUN dnf install -y sssd
RUN dnf install -y realmd
RUN dnf install -y which
RUN dnf install -y credentials-fetcher

RUN mkdir -p /var/credentials-fetcher/krbdir
RUN mkdir -p /var/credentials-fetcher/logging
RUN mkdir -p /var/credentials-fetcher/socket

VOLUME ["/krb5","/etc/krb5.conf.d","/var/scratch"]

RUN cp /usr/sbin/krb5.conf /etc/krb5.conf

CMD ["/usr/sbin/credentials-fetcherd"]
