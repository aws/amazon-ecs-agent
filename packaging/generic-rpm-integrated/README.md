##Steps to build and install package:

* Install build and install dependencies
  ```
  rpm-build make golang-go gcc git wget rpmdevtools 
  ```
* Download the latest agent
  ```
  curl -O https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-v${VERSION}.tar
  ```
  or
  ```
  curl -O https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-arm64-v${VERSION}.tar
  ```
* Build the package by running 
  ```
  make generic-rpm
  ```
* Install docker via docker repos: https://docs.docker.com/engine/install/
* Install the package using built `.rpm` file from previous step
* (Only needed if EFS is needed) Install `efs-utils` following instructions on https://docs.aws.amazon.com/efs/latest/ug/installing-amazon-efs-utils.html#installing-other-distro
* Start and enable ecs service
  ```
  sudo systemctl enable --now ecs
  ```
* (Only needed if EFS is needed) Start and enable ecs volume plugin 
  ```
  sudo systemctl enable --now amazon-ecs-volume-plugin
  ```

