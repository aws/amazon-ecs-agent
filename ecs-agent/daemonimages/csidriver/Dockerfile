FROM public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base-csi-ebs:latest.2 AS linux-amazon
COPY csi-driver /bin/aws-ebs-csi-driver
ENTRYPOINT ["/bin/aws-ebs-csi-driver"]


