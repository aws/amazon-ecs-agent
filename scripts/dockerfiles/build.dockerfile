FROM public.ecr.aws/amazonlinux/amazonlinux:2
RUN yum update -y && yum install -y golang make tar rpm-build
WORKDIR /workspace/amazon-ecs-init
# we're going to run the build as the non-privileged user, so we need write access to the directory
RUN chmod -R 777 /workspace
CMD /bin/bash -c 'make rpm'
