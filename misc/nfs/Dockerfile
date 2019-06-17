FROM amazonlinux:2

# TODO: Make this clean up nfsd processes
RUN yum update -y && yum install -y nfs-utils
COPY nfs.sh /nfs.sh
CMD "/nfs.sh"
