FROM public.ecr.aws/appmesh/aws-appmesh-envoy:v1.20.0.1-prod
WORKDIR /
COPY envoy_config/* /etc/envoy/
COPY start.sh /usr/bin/start.sh
RUN chmod +x /usr/bin/start.sh
# Install packages.
RUN yum -y install jq
RUN yum -y install iptables

ENTRYPOINT /usr/bin/start.sh
