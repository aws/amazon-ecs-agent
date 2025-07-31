## Steps to build and install Amazon Linux ECS Init RPM:

### Prerequisites

* An **Amazon Linux instance** - This package is specifically designed for Amazon Linux distributions.
* **Docker** - Docker must be installed and running. You can skip this step if you're using [ECS Optimized AMIs](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-optimized_AMI.html), since they come with Docker pre-installed and running at startup. 
  ```bash
  # Query the available versions using "yum info docker", and install the latest available version.
  # Example:
  sudo yum install -y docker-25.0.8
  ```

* Install required build tools and dependencies:
  ```bash
  sudo yum install -y rpm-build make gcc git wget rpmdevtools
  ```

* Check out this amazon-ecs-agent package locally on your Amazon Linux instance.

* **Go Language** - The build process requires Go:
  ```bash
  # Query the available versions using "yum info golang", and install the latest available version.
  # Example:
  sudo yum install -y golang-1.24.5
  ```

### Build

1. **Build the Amazon Linux RPM**:
   ```bash
   make amazon-linux-rpm-codebuild
   ```

2. **Locate the built RPM**:
   The RPM will be created in the current directory with the naming pattern:
   - For x86_64: `ecs-init-{VERSION}-1.{amzn_version}.x86_64.rpm`
   - For aarch64: `ecs-init-{VERSION}-1.{amzn_version}.aarch64.rpm` 

### Install

1. **Stop the existing ECS services** (if running):
   ```bash
   sudo systemctl stop ecs
   sudo systemctl stop amazon-ecs-volume-plugin
   ```

2. **Remove existing ECS Init package** (if installed):
   ```bash
   sudo rpm -e ecs-init
   ```

3. **Install the ECS Init RPM built above**:
   ```bash
   sudo rpm -ivh ecs-init-{VERSION}-1.{amzn_version}.{architecture}.rpm
   ```

4. **(Optional) Install EFS Utils** if EFS support is needed:
   
   ECS AMIs come with EFS utils installed, so you can skip this step if you're using an ECS AMI.
   
   Otherwise, Follow instructions at: https://docs.aws.amazon.com/efs/latest/ug/installing-amazon-efs-utils.html#installing-other-distro for installation. 

5. **Start and enable ECS services**:
   ```bash
   sudo systemctl enable --now ecs
   sudo systemctl enable --now amazon-ecs-volume-plugin
   ```