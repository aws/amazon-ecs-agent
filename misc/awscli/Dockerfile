FROM debian:stable

RUN apt-get update && apt-get install -y \
    python2.7 curl

RUN curl -O https://bootstrap.pypa.io/get-pip.py

RUN python2.7 get-pip.py && pip install awscli
