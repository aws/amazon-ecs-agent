FROM public.ecr.aws/docker/library/debian:stable-20240110-slim

RUN apt-get update &&  \
    apt-get install -y ca-certificates && \
    rm -rf /var/lib/apt/lists/*
