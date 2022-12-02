##Steps to build and install package:

* Install build and install dependencies
  ```
  rpm-build make gcc git wget rpmdevtools 
  ```
* Build the package by running 
  ```
  make generic-rpm-integrated
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
