# Build csi-driver from the minimal eks base to add required mount utils
# TODO update Dockerfile with arch to build arm and windows

FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-csi-ebs:latest.2 AS linux-amazon
COPY csi-driver /bin/aws-ebs-csi-driver
ENTRYPOINT ["/bin/aws-ebs-csi-driver"]


