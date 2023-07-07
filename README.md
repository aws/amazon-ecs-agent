# Amazon ECS Container Agent

![Amazon ECS logo](doc/ecs.png "Amazon ECS")

The Amazon ECS Container Agent is a component of Amazon Elastic Container Service
([Amazon ECS](http://aws.amazon.com/ecs/)) and is responsible for managing containers on behalf of Amazon ECS.

This repository comes with ECS-Init, which is a [systemd](http://www.freedesktop.org/wiki/Software/systemd/) based service to support the Amazon ECS Container Agent and keep it running. It is used for systems that utilize `systemd` as init systems and is packaged as deb or rpm. The source for ECS-Init is available in this repository at `./ecs-init` while the packaging is available at `./packaging`.

## Usage

The best source of information on running this software is the
[Amazon ECS documentation](http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ECS_agent.html).

### On the Amazon Linux AMI

On the [Amazon Linux AMI](https://aws.amazon.com/amazon-linux-ami/), we provide an installable RPM which can be used via
`sudo yum install ecs-init && sudo start ecs`. This is the recommended way to run it in this environment.

### On Other Linux AMIs

[Amazon ECS docs](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-agent-install.html) provides deb and rpm packages and instructions to install ECS Container Agent on non-Amazon Linux instances.

The Amazon ECS Container Agent may also be run in a Docker container on an EC2 instance with a recent Docker version
installed. Docker images are available in
[Docker Hub Repository](https://hub.docker.com/r/amazon/amazon-ecs-agent) and [ECR Public Gallery](https://gallery.ecr.aws/ecs/amazon-ecs-agent).

```bash
$ # Set up directories the agent uses
$ mkdir -p /var/log/ecs /etc/ecs /var/lib/ecs/data
$ touch /etc/ecs/ecs.config
$ # Set up necessary rules to enable IAM roles for tasks
$ sysctl -w net.ipv4.conf.all.route_localnet=1
$ iptables -t nat -A PREROUTING -p tcp -d 169.254.170.2 --dport 80 -j DNAT --to-destination 127.0.0.1:51679
$ iptables -t nat -A OUTPUT -d 169.254.170.2 -p tcp -m tcp --dport 80 -j REDIRECT --to-ports 51679
$ # Run the agent
$ docker run --name ecs-agent \
    --detach=true \
    --restart=on-failure:10 \
    --volume=/var/run/docker.sock:/var/run/docker.sock \
    --volume=/var/log/ecs:/log \
    --volume=/var/lib/ecs/data:/data \
    --net=host \
    --env-file=/etc/ecs/ecs.config \
    --env=ECS_LOGFILE=/log/ecs-agent.log \
    --env=ECS_DATADIR=/data/ \
    --env=ECS_ENABLE_TASK_IAM_ROLE=true \
    --env=ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST=true \
    amazon/amazon-ecs-agent:latest
```

### On Other Linux AMIs when awsvpc networking mode is enabled

For the AWS VPC networking mode, ECS agent requires CNI plugin and dhclient to be available. ECS also needs the ecs-init to run as part of its startup.
The following is an example of docker run configuration for running ecs-agent with Task ENI enabled. Note that ECS agent currently only supports cgroupfs for cgroup driver.
```
$ # Run the agent
$ /usr/bin/docker run --name ecs-agent \
--init \
--restart=on-failure:10 \
--volume=/var/run:/var/run \
--volume=/var/log/ecs/:/log:Z \
--volume=/var/lib/ecs/data:/data:Z \
--volume=/etc/ecs:/etc/ecs \
--volume=/sbin:/host/sbin \
--volume=/lib:/lib \
--volume=/lib64:/lib64 \
--volume=/usr/lib:/usr/lib \
--volume=/usr/lib64:/usr/lib64 \
--volume=/proc:/host/proc \
--volume=/sys/fs/cgroup:/sys/fs/cgroup \
--net=host \
--env-file=/etc/ecs/ecs.config \
--cap-add=sys_admin \
--cap-add=net_admin \
--env ECS_ENABLE_TASK_ENI=true \
--env ECS_UPDATES_ENABLED=true \
--env ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION=1h \
--env ECS_DATADIR=/data \
--env ECS_ENABLE_TASK_IAM_ROLE=true \
--env ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST=true \
--env ECS_LOGFILE=/log/ecs-agent.log \
--env ECS_AVAILABLE_LOGGING_DRIVERS='["json-file","awslogs","syslog","none"]' \
--env ECS_LOGLEVEL=info \
--detach \
amazon/amazon-ecs-agent:latest
```

See also the Advanced Usage section below.

### On the ECS Optimized Windows AMI

ECS Optimized Windows AMI ships with a pre-installed PowerShell module called ECSTools to install, configure, and run the ECS Agent as a Windows service.
To install the service, you can run the following PowerShell commands on an EC2 instance. To launch into another cluster instead of windows, replace the 'windows' in the script below with the name of your cluster.

```powershell
PS C:\> Import-Module ECSTools
PS C:\> # The -EnableTaskIAMRole option is required to enable IAM roles for tasks.
PS C:\> Initialize-ECSAgent -Cluster 'windows' -EnableTaskIAMRole
```

#### Downloading Different Version of ECS Agent

To download different version of ECS Agent, you can do the following:

```powershell
PS C:\> # use agentVersion = "latest" for the latest available agent version
PS C:\> $agentVersion = "v1.20.4"
PS C:\> Initialize-ECSAgent -Cluster 'windows' -EnableTaskIAMRole -Version $agentVersion
```

## Build ECS Agent from source

### Build ECS Agent Image (Linux)

ECS Agent can also be built locally from source on a linux machine. Use the following steps to build ECS Agent
* Get ECS Agent source
```
git clone https://github.com/aws/amazon-ecs-agent.git
```
* Build Agent image using ```release-agent``` make target
```
make release-agent
```
This installs the required build dependencies, builds ECS Agent image and saves it at a path ```ecs-agent-v${AGENT_VERSION}.tar```. Load this using
```
docker load < ecs-agent-v${AGENT_VERSION}.tar
```
Follow the instructions [above](https://github.com/aws/amazon-ecs-agent#on-other-linux-amis) to continue with the installation

### Build and run standalone (Linux)

The Amazon ECS Container Agent may also be run outside of a Docker container as a Go binary. At this time, this is not recommended
for production on Linux, but it can be useful for development or easier integration with your local Go tools.

The following commands run the agent outside of Docker:

```
make gobuild
./out/amazon-ecs-agent
```

### Standalone (Windows)

The Amazon ECS Container Agent may be built by invoking `scripts\build_agent.ps1`

### Scripts (Windows)

The following scripts are available to help develop the Amazon ECS Container Agent on Windows:

* `scripts\run-integ-tests.ps1` - Runs all integration tests in the `engine` and `stats` packages
* `misc\windows-deploy\Install-ECSAgent.ps1` - Install the ECS agent as a Windows service
* `misc\windows-deploy\amazon-ecs-agent.ps1` - Helper script to set up the host and run the agent as a process
* `misc\windows-deploy\user-data.ps1` - Sample user-data that can be used with the Windows Server 2016 with Containers
  AMI to run the agent as a process


### Build ECS-Init Package (Linux)

ECS-Init package can also be built as a deb or rpm depending on the linux system you are running. Follow instructions at [generic-deb-integrated](https://github.com/aws/amazon-ecs-agent/tree/master/packaging/generic-deb-integrated/debian) or [generic-rpm-integrated](https://github.com/aws/amazon-ecs-agent/tree/master/packaging/generic-rpm-integrated) to build and install ECS Agent with Init using deb or rpm.

## Advanced Usage

The Amazon ECS Container Agent supports a number of configuration options, most of which should be set through
environment variables.

### Environment Variables

The table below provides an overview of optional environment variables that can be used to configure the ECS agent. See
[the Amazon ECS developer guide](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-agent-config.html) for
additional details on each available environment variable.

| Environment Key | Example Value(s)            | Description | Default value on Linux | Default value on Windows |
|:----------------|:----------------------------|:------------|:-----------------------|:-------------------------|
| `ECS_CLUSTER`       | clusterName             | The cluster this agent should check into. | default | default |
| `ECS_RESERVED_PORTS` | `[22, 80, 5000, 8080]` | An array of ports that should be marked as unavailable for scheduling on this container instance. | `[22, 2375, 2376, 51678, 51679]` | `[53, 135, 139, 445, 2375, 2376, 3389, 5985, 5986, 51678, 51679]`
| `ECS_RESERVED_PORTS_UDP` | `[53, 123]` | An array of UDP ports that should be marked as unavailable for scheduling on this container instance. | `[]` | `[]` |
| `ECS_ENGINE_AUTH_TYPE`     |  "docker" &#124; "dockercfg" | The type of auth data that is stored in the `ECS_ENGINE_AUTH_DATA` key. | | |
| `ECS_ENGINE_AUTH_DATA`     | See the [dockerauth documentation](https://godoc.org/github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerauth) | Docker [auth data](https://godoc.org/github.com/aws/amazon-ecs-agent/agent/dockerclient/dockerauth) formatted as defined by `ECS_ENGINE_AUTH_TYPE`. | | |
| `AWS_DEFAULT_REGION` | &lt;us-west-2&gt;&#124;&lt;us-east-1&gt;&#124;&hellip; | The region to be used in API requests as well as to infer the correct backend host. | Taken from Amazon EC2 instance metadata. | Taken from Amazon EC2 instance metadata. |
| `AWS_ACCESS_KEY_ID` | AKIDEXAMPLE             | The [access key](http://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html) used by the agent for all calls. | Taken from Amazon EC2 instance metadata. | Taken from Amazon EC2 instance metadata. |
| `AWS_SECRET_ACCESS_KEY` | EXAMPLEKEY | The [secret key](http://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html) used by the agent for all calls. | Taken from Amazon EC2 instance metadata. | Taken from Amazon EC2 instance metadata. |
| `AWS_SESSION_TOKEN` | | The [session token](http://docs.aws.amazon.com/STS/latest/UsingSTS/Welcome.html) used for temporary credentials. | Taken from Amazon EC2 instance metadata. | Taken from Amazon EC2 instance metadata. |
| `DOCKER_HOST`   | `unix:///var/run/docker.sock` | Used to create a connection to the Docker daemon; behaves similarly to this environment variable as used by the Docker client. | `unix:///var/run/docker.sock` | `npipe:////./pipe/docker_engine` |
| `ECS_LOGLEVEL`  | &lt;crit&gt; &#124; &lt;error&gt; &#124; &lt;warn&gt; &#124; &lt;info&gt; &#124; &lt;debug&gt; | The level of detail to be logged. | info | info |
| `ECS_LOGLEVEL_ON_INSTANCE`  | &lt;none&gt; &#124; &lt;crit&gt; &#124; &lt;error&gt; &#124; &lt;warn&gt; &#124; &lt;info&gt; &#124; &lt;debug&gt; | Can be used to override `ECS_LOGLEVEL` and set a level of detail that should be logged in the on-instance log file, separate from the level that is logged in the logging driver. If a logging driver is explicitly set, on-instance logs are turned off by default, but can be turned back on with this variable. | none if `ECS_LOG_DRIVER` is explicitly set to a non-empty value; otherwise the same value as `ECS_LOGLEVEL` | none if `ECS_LOG_DRIVER` is explicitly set to a non-empty value; otherwise the same value as `ECS_LOGLEVEL` |
| `ECS_LOGFILE`   | /ecs-agent.log              | The location where logs should be written. Log level is controlled by `ECS_LOGLEVEL`. | blank | blank |
| `ECS_CHECKPOINT`   | &lt;true &#124; false&gt; | Whether to checkpoint state to the DATADIR specified below. | true if `ECS_DATADIR` is explicitly set to a non-empty value; false otherwise | true if `ECS_DATADIR` is explicitly set to a non-empty value; false otherwise |
| `ECS_DATADIR`      |   /data/                  | The container path where state is checkpointed for use across agent restarts. Note that on Linux, when you specify this, you will need to make sure that the Agent container has a bind mount of `$ECS_HOST_DATA_DIR/data:$ECS_DATADIR` with the corresponding values of `ECS_HOST_DATA_DIR` and `ECS_DATADIR`. | /data/ | `C:\ProgramData\Amazon\ECS\data`
| `ECS_UPDATES_ENABLED` | &lt;true &#124; false&gt; | Whether to exit for an updater to apply updates when requested. | false | false |
| `ECS_DISABLE_METRICS`     | &lt;true &#124; false&gt;  | Whether to disable metrics gathering for tasks. | false | false |
| `ECS_POLL_METRICS`     | &lt;true &#124; false&gt;  | Whether to poll or stream when gathering metrics for tasks. Setting this value to `true` can help reduce the CPU usage of dockerd and containerd on the ECS container instance. See also ECS_POLL_METRICS_WAIT_DURATION for setting the poll interval. | `false` | `false` |
| `ECS_POLLING_METRICS_WAIT_DURATION` | 10s | Time to wait between polling for metrics for a task. Not used when ECS_POLL_METRICS is false. Maximum value is 20s and minimum value is 5s. If user sets above maximum it will be set to max, and if below minimum it will be set to min. As the number of tasks/containers increase, a higher `ECS_POLLING_METRICS_WAIT_DURATION` value can potentially cause a problem where memory reservation value of ECS cluster reported in metrics becomes unstable due to missing metrics sample at metric collection time. It is recommended to keep this value smaller than 18s. This behavior is only observed on certain OS and platforms. | 10s | 10s |
| `ECS_PULL_DEPENDENT_CONTAINERS_UPFRONT` | &lt;true &#124; false&gt; | Whether to pull images for containers with dependencies before the dependsOn condition has been satisfied. | false | false |
| `ECS_RESERVED_MEMORY` | 32 | Reduction, in MiB, of the memory capacity of the instance that is reported to Amazon ECS. Used by Amazon ECS when placing tasks on container instances. This doesn't reserve memory usage on the instance. | 0 | 0 |
| `ECS_AVAILABLE_LOGGING_DRIVERS` | `["awslogs","fluentd","gelf","json-file","journald","logentries","splunk","syslog"]` | Which logging drivers are available on the container instance. | `["json-file","none"]` | `["json-file","none"]` |
| `ECS_DISABLE_PRIVILEGED` | `true` | Whether launching privileged containers is disabled on the container instance. | `false` | `false` |
| `ECS_SELINUX_CAPABLE` | `true` | Whether SELinux is available on the container instance. (Limited support; Z-mode mounts only.) | `false` | `false` |
| `ECS_APPARMOR_CAPABLE` | `true` | Whether AppArmor is available on the container instance. | `false` | `false` |
| `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION` | 10m | Default time to wait to delete containers for a stopped task (see also `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION_JITTER`). If set to less than 1 second, the value is ignored.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    | 3h | 3h |
| `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION_JITTER` | 1h | Jitter value for the task engine cleanup wait duration. When specified, the actual cleanup wait duration time for each task will be the duration specified in `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION` plus a random duration between 0 and the jitter duration. | blank | blank |
| `ECS_CONTAINER_STOP_TIMEOUT` | 10m | Instance scoped configuration for time to wait for the container to exit normally before being forcibly killed. | 30s | 30s |
| `ECS_CONTAINER_START_TIMEOUT` | 10m | Timeout before giving up on starting a container. | 3m | 8m |
| `ECS_CONTAINER_CREATE_TIMEOUT` | 10m | Timeout before giving up on creating a container. Minimum value is 1m. If user sets a value below minimum it will be set to min. | 4m | 4m |
| `ECS_ENABLE_TASK_IAM_ROLE` | `true` | Whether to enable IAM Roles for Tasks on the Container Instance | `false` | `false` |
| `ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST` | `true` | Whether to enable IAM Roles for Tasks when launched with `host` network mode on the Container Instance | `false` | `false` |
| `ECS_DISABLE_IMAGE_CLEANUP` | `true` | Whether to disable automated image cleanup for the ECS Agent. | `false` | `false` |
| `ECS_IMAGE_CLEANUP_INTERVAL` | 30m | The time interval between automated image cleanup cycles. If set to less than 10 minutes, the value is ignored. | 30m | 30m |
| `ECS_IMAGE_MINIMUM_CLEANUP_AGE` | 30m | The minimum time interval between when an image is pulled and when it can be considered for automated image cleanup. | 1h | 1h |
| `NON_ECS_IMAGE_MINIMUM_CLEANUP_AGE` | 30m | The minimum time interval between when a non ECS image is created and when it can be considered for automated image cleanup. | 1h | 1h |
| `ECS_NUM_IMAGES_DELETE_PER_CYCLE` | 5 | The maximum number of images to delete in a single automated image cleanup cycle. If set to less than 1, the value is ignored. | 5 | 5 |
| `ECS_IMAGE_PULL_BEHAVIOR` | &lt;default &#124; always &#124; once &#124; prefer-cached &gt; | The behavior used to customize the pull image process. If `default` is specified, the image will be pulled remotely, if the pull fails then the cached image in the instance will be used. If `always` is specified, the image will be pulled remotely, if the pull fails then the task will fail. If `once` is specified, the image will be pulled remotely if it has not been pulled before or if the image was removed by image cleanup, otherwise the cached image in the instance will be used. If `prefer-cached` is specified, the image will be pulled remotely if there is no cached image, otherwise the cached image in the instance will be used. | default | default |
| `ECS_IMAGE_PULL_INACTIVITY_TIMEOUT` | 1m | The time to wait after docker pulls complete waiting for extraction of a container. Useful for tuning large Windows containers. | 1m | 3m |
| `ECS_IMAGE_PULL_TIMEOUT` | 1h | The time to wait for pulling docker image. | 2h | 2h |
| `ECS_INSTANCE_ATTRIBUTES` | `{"stack": "prod"}` | These attributes take effect only during initial registration. After the agent has joined an ECS cluster, use the PutAttributes API action to add additional attributes. For more information, see [Amazon ECS Container Agent Configuration](http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-agent-config.html) in the Amazon ECS Developer Guide.| `{}` | `{}` |
| `ECS_ENABLE_TASK_ENI` | `false` | Whether to enable task networking for task to be launched with its own network interface | `false` | Not applicable |
| `ECS_ENABLE_HIGH_DENSITY_ENI` | `false` | Whether to enable high density eni feature when using task networking | `true` | Not applicable |
| `ECS_CNI_PLUGINS_PATH` | `/ecs/cni` | The path where the cni binary file is located | `/amazon-ecs-cni-plugins` | Not applicable |
| `ECS_AWSVPC_BLOCK_IMDS` | `true` | Whether to block access to [Instance Metadata](http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-metadata.html) for Tasks started with `awsvpc` network mode | `false` | Not applicable |
| `ECS_AWSVPC_ADDITIONAL_LOCAL_ROUTES` | `["10.0.15.0/24"]` | In `awsvpc` network mode, traffic to these prefixes will be routed via the host bridge instead of the task ENI | `[]` | Not applicable |
| `ECS_ENABLE_CONTAINER_METADATA` | `true` | When `true`, the agent will create a file describing the container's metadata and the file can be located and consumed by using the container enviornment variable `$ECS_CONTAINER_METADATA_FILE` | `false` | `false` |
| `ECS_HOST_DATA_DIR` | `/var/lib/ecs` | The source directory on the host from which ECS_DATADIR is mounted. We use this to determine the source mount path for container metadata files in the case the ECS Agent is running as a container. We do not use this value in Windows because the ECS Agent is not running as container in Windows. On Linux, note that when you specify this, you will need to make sure that the Agent container has a bind mount of `$ECS_HOST_DATA_DIR/data:$ECS_DATADIR` with the corresponding values of `ECS_HOST_DATA_DIR` and `ECS_DATADIR`. | `/var/lib/ecs` | `Not used` |
| `ECS_ENABLE_TASK_CPU_MEM_LIMIT` | `true` | Whether to enable task-level cpu and memory limits | `true` | `false` |
| `ECS_CGROUP_PATH` | `/sys/fs/cgroup` | The root cgroup path that is expected by the ECS agent. This is the path that accessible from the agent mount. | `/sys/fs/cgroup` | Not applicable |
| `ECS_CGROUP_CPU_PERIOD` | `10ms` | CGroups CPU period for task level limits. This value should be between 8ms to 100ms | `100ms` | Not applicable |
| `ECS_AGENT_HEALTHCHECK_HOST` | `localhost` | Override for the ecs-agent container's healthcheck localhost ip address| `localhost` | `localhost` |
| `ECS_ENABLE_CPU_UNBOUNDED_WINDOWS_WORKAROUND` | `true` | When `true`, ECS will allow CPU unbounded(CPU=`0`) tasks to run along with CPU bounded tasks in Windows. | Not applicable | `false` |
| `ECS_ENABLE_MEMORY_UNBOUNDED_WINDOWS_WORKAROUND` | `true` | When `true`, ECS will ignore the memory reservation parameter (soft limit) to run along with memory bounded tasks in Windows. To run a memory unbounded task, omit the memory hard limit and set any memory reservation, it will be ignored. | Not applicable | `false` |
| `ECS_TASK_METADATA_RPS_LIMIT` | `100,150` | Comma separated integer values for steady state and burst throttle limits for combined total traffic to task metadata endpoint and agent api endpoint. | `40,60` | `40,60` |
| `ECS_SHARED_VOLUME_MATCH_FULL_CONFIG` | `true` | When `true`, ECS Agent will compare name, driver options, and labels to make sure volumes are identical. When `false`, Agent will short circuit shared volume comparison if the names match. This is the default Docker behavior. If a volume is shared across instances, this should be set to `false`. | `false` | `false`|
| `ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM` | `ec2_instance` | If `ec2_instance` is specified, existing tags defined on the container instance will be registered to Amazon ECS and will be discoverable using the `ListTagsForResource` API. Using this requires that the IAM role associated with the container instance have the `ec2:DescribeTags` action allowed. | `none` | `none` |
| `ECS_CONTAINER_INSTANCE_TAGS` | `{"tag_key": "tag_val"}` | The metadata that you apply to the container instance to help you categorize and organize them. Each tag consists of a key and an optional value, both of which you define. Tag keys can have a maximum character length of 128 characters, and tag values can have a maximum length of 256 characters. If tags also exist on your container instance that are propagated using the `ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM` parameter, those tags will be overwritten by the tags specified using `ECS_CONTAINER_INSTANCE_TAGS`. | `{}` | `{}` |
| `ECS_ENABLE_UNTRACKED_IMAGE_CLEANUP` | `true` | Whether to allow the ECS agent to delete containers and images that are not part of ECS tasks. | `false` | `false` |
| `ECS_EXCLUDE_UNTRACKED_IMAGE` | `alpine:latest` | Comma separated list of `imageName:tag` of images that should not be deleted by the ECS agent if `ECS_ENABLE_UNTRACKED_IMAGE_CLEANUP` is enabled. | | |
| `ECS_DISABLE_DOCKER_HEALTH_CHECK` | `false` | Whether to disable the Docker Container health check for the ECS Agent. | `false` | `false` |
| `ECS_NVIDIA_RUNTIME` | nvidia | The Nvidia Runtime to be used to pass Nvidia GPU devices to containers. | nvidia | Not Applicable |
| `ECS_ALTERNATE_CREDENTIAL_PROFILE` | default | An alternate credential role/profile name. | default | default |
| `ECS_ENABLE_SPOT_INSTANCE_DRAINING` | `true` | Whether to enable Spot Instance draining for the container instance. If true, if the container instance receives a [spot interruption notice](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/spot-interruptions.html), agent will set the instance's status to [DRAINING](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/container-instance-draining.html), which gracefully shuts down and replaces all tasks running on the instance that are part of a service. It is recommended that this be set to `true` when using spot instances. | `false` | `false` |
| `ECS_LOG_ROLLOVER_TYPE` | `size` &#124; `hourly` | Determines whether the container agent logfile will be rotated based on size or hourly. By default, the agent logfile is rotated each hour. | `hourly` | `hourly` |
| `ECS_LOG_OUTPUT_FORMAT` | `logfmt` &#124; `json` | Determines the log output format. When the json format is used, each line in the log would be a structured JSON map. | `logfmt` | `logfmt` |
| `ECS_LOG_MAX_FILE_SIZE_MB` | `10` | When the ECS_LOG_ROLLOVER_TYPE variable is set to size, this variable determines the maximum size (in MB) the log file before it is rotated. If the rollover type is set to hourly then this variable is ignored. | `10` | `10` |
| `ECS_LOG_MAX_ROLL_COUNT` | `24` | Determines the number of rotated log files to keep. Older log files are deleted once this limit is reached. | `24` | `24` |
| `ECS_LOG_DRIVER` | `awslogs` &#124; `fluentd` &#124; `gelf` &#124; `json-file` &#124; `journald` &#124; `logentries` &#124; `syslog` &#124; `splunk` | The logging driver to be used by the Agent container. | `json-file` | Not applicable |
| `ECS_LOG_OPTS` | `{"option":"value"}` | The options for configuring the logging driver set in `ECS_LOG_DRIVER`. | `{}` | Not applicable |
| `ECS_ENABLE_AWSLOGS_EXECUTIONROLE_OVERRIDE` | `true` | Whether to enable awslogs log driver to authenticate via credentials of task execution IAM role. Needs to be true if you want to use awslogs log driver in a task that has task execution IAM role specified. When using the ecs-init RPM with version equal or later than V1.16.0-1, this env is set to true by default. | `false` | `false` |
| `ECS_FSX_WINDOWS_FILE_SERVER_SUPPORTED` | `true` | Whether FSx for Windows File Server volume type is supported on the container instance. This variable is only supported on agent versions 1.47.0 and later. | `false` | `true` |
| `ECS_ENABLE_RUNTIME_STATS` | `true` | Determines if [pprof](https://pkg.go.dev/net/http/pprof) is enabled for the agent. If enabled, the different profiles can be accessed through the agent's introspection port (e.g. `curl http://localhost:51678/debug/pprof/heap > heap.pprof`). In addition, agent's [runtime stats](https://pkg.go.dev/runtime#ReadMemStats) are logged to `/var/log/ecs/runtime-stats.log` file. | `false` | `false` |
| `ECS_EXCLUDE_IPV6_PORTBINDING` | `true` | Determines if agent should exclude IPv6 port binding using default network mode. If enabled, IPv6 port binding will be filtered out, and the response of DescribeTasks API call will not show tasks' IPv6 port bindings, but it is still included in Task metadata endpoint. | `true` | `true` |
| `ECS_WARM_POOLS_CHECK` | `true` | Whether to ensure instances going into an [EC2 Auto Scaling group warm pool](https://docs.aws.amazon.com/autoscaling/ec2/userguide/ec2-auto-scaling-warm-pools.html) are prevented from being registered with the cluster. Set to true only if using EC2 Autoscaling | `false` | `false` |
| `ECS_SKIP_LOCALHOST_TRAFFIC_FILTER` | `false` | By default, the ecs-init service adds an iptable rule to drop non-local packets to localhost if they're not part of an existing forwarded connection or DNAT, and removes the rule upon stop. If this is set to true, the rule will not be added or removed. | `false` | `false` |
| `ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS` | `true` | By default, the ecs-init service adds an iptable rule to block access to the agent introspection port from off-host (or containers in awsvpc network mode), and removes the rule upon stop. If this is set to true, the rule will not be added or removed | `false` | `false` |
| `ECS_OFFHOST_INTROSPECTION_INTERFACE_NAME` | `eth0` | The primary network interface name to be used for blocking offhost agent introspection port access | `eth0` | `eth0` |
| `ECS_ENABLE_GPU_SUPPORT` | `true` | Whether you use container instances with GPU support. This parameter is specified for the agent. You must also configure your task definitions for GPU. For more information | `false` | `Not applicable` |
| `HTTP_PROXY` | `10.0.0.131:3128` | The hostname (or IP address) and port number of an HTTP proxy to use for the Amazon ECS agent to connect to the internet. For example, this proxy will be used if your container instances do not have external network access through an Amazon VPC internet gateway or NAT gateway or instance. If this variable is set, you must also set the NO_PROXY variable to filter Amazon EC2 instance metadata and Docker daemon traffic from the proxy. | `null` | `null` |
| `NO_PROXY` | <For Linux: 169.254.169.254,169.254.170.2,/var/run/docker.sock &#124; For Windows: 169.254.169.254,169.254.170.2,\\.\pipe\docker_engine> | The HTTP traffic that should not be forwarded to the specified HTTP_PROXY. You must specify 169.254.169.254,/var/run/docker.sock to filter Amazon EC2 instance metadata and Docker daemon traffic from the proxy. | `null` | `null` |
| `ECS_GMSA_SUPPORTED` | `true` | Whether you use gMSA authentication to Active Directory in tasks. Each task must specify the location of a credential specification file in the `dockerSecurityOpts` parameter of a container definition. On Linux, this requires the [credentials-fetcher daemon](https://github.com/aws/credentials-fetcher). | `false` | `false` |
| `CREDENTIALS_FETCHER_HOST`   | `unix:///var/credentials-fetcher/socket/credentials_fetcher.sock` | Used to create a connection to the [credentials-fetcher daemon](https://github.com/aws/credentials-fetcher); to support gMSA on Linux. The default is fine for most users, only needs to be modified if user is configuring a custom credentials-fetcher socket path, ie, [CF_UNIX_DOMAIN_SOCKET_DIR](https://github.com/aws/credentials-fetcher#default-environment-variables). | `unix:///var/credentials-fetcher/socket/credentials_fetcher.sock` | Not Applicable |
| `CREDENTIALS_FETCHER_SECRET_NAME_FOR_DOMAINLESS_GMSA`   | `secretmanager-secretname` | Used to support scaling option for gMSA on Linux [credentials-fetcher daemon](https://github.com/aws/credentials-fetcher). If user is configuring gMSA on a non-domain joined instance, they need to create an Active Directory user with access to retrieve principals for the gMSA account and store it in secrets manager | `secretmanager-secretname` | Not Applicable |
| `ECS_DYNAMIC_HOST_PORT_RANGE` | `100-200` | This specifies the dynamic host port range that the agent uses to assign host ports from, for container ports mapping. If there are no available ports in the range for containers, including customer containers and Service Connect Agent containers (if Service Connect is enabled), service deployments would fail. | Defined by `/proc/sys/net/ipv4/ip_local_port_range` | `49152-65535` |
| `ECS_TASK_PIDS_LIMIT` | `100` | Specifies the per-task pids limit cgroup setting for each task launched on the container instance. This setting maps to the pids.max cgroup setting at the ECS task level. See https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html#pid. If unset, pids will be unlimited. Min value is 1 and max value is 4194304 (4*1024*1024) | `unset` | Not Supported on Windows |

Additionally, the following environment variable(s) can be used to configure the behavior of the ecs-init service. When using ECS-Init, all env variables, including the ECS Agent variables above, are read from path `/etc/ecs/ecs.config`:
| Environment Variable Name | Example Value(s)            | Description | Default value |
|:----------------|:----------------------------|:------------|:-----------------------|
| `ECS_SKIP_LOCALHOST_TRAFFIC_FILTER` | &lt;true &#124; false&gt; | By default, the ecs-init service adds an iptable rule to drop non-local packets to localhost if they're not part of an existing forwarded connection or DNAT, and removes the rule upon stop. If `ECS_SKIP_LOCALHOST_TRAFFIC_FILTER` is set to true, this rule will not be added/removed. | false |
| `ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS` | &lt;true &#124; false&gt; | By default, the ecs-init service adds an iptable rule to block access to ECS Agent's introspection port from off-host (or containers in awsvpc network mode), and removes the rule upon stop. If `ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS` is set to true, this rule will not be added/removed. | false |
| `ECS_OFFHOST_INTROSPECTION_INTERFACE_NAME` | `eth0` | Primary network interface name to be used for blocking offhost agent introspection port access. By default, this value is `eth0` | `eth0` |
| `ECS_AGENT_LABELS` | `{"test.label.1":"value1","test.label.2":"value2"}` | The labels to add to the ECS Agent container. | |



### Persistence

When you run the Amazon ECS Container Agent in production, its `datadir` should be persisted between runs of the Docker
container. If this data is not persisted, the agent registers a new container instance ARN on each launch and is not
able to update the state of tasks it previously ran.

### Flags

The agent also supports the following flags:

* `-k` &mdash; The agent will not require valid SSL certificates for the services that it communicates with. We
  recommend against using this flag.
* ` -loglevel` &mdash; Options: `[<crit>|<error>|<warn>|<info>|<debug>]`. The agent will output on stdout at the given
  level. This is overridden by the `ECS_LOGLEVEL` environment variable, if present.


### Make Targets (on Linux)

The following targets are available. Each may be run with `make <target>`.

| Make Target            | Description |
|:-----------------------|:------------|
| `release-agent`        | *(Default Agent build)* Builds Agent fetching required dependencies and saves image .tar to disk|
| `generic-rpm-integrated`| Builds init rpm package and saves .rpm package to disk |
| `generic-deb-integrated`| Builds init deb package and saves .deb package to disk |
| `release`              | *(Legacy Agent build)* Builds the agent within a Docker container and packages it into a scratch-based image |
| `gobuild`              | Runs a normal `go build` of the agent and stores the binary in `./out/amazon-ecs-agent` |
| `static`               | Runs `go build` to produce a static binary in `./out/amazon-ecs-agent` |
| `test`                 | Runs all unit tests using `go test` |
| `test-in-docker`       | Runs all tests inside a Docker container |
| `run-integ-tests`      | Runs all integration tests in the `engine` and `stats` packages |
| `clean`                | Removes build artifacts. *Note: this does not remove Docker images* |


## Contributing

Contributions and feedback are welcome! Proposals and pull requests will be considered and responded to. For more
information, see the [CONTRIBUTING.md](https://github.com/aws/amazon-ecs-agent/blob/master/CONTRIBUTING.md) file.

If you have a bug/and issue around the behavior of the ECS agent, please open it here.

If you have a feature request, please open it over at the [AWS Containers Roadmap](https://github.com/aws/containers-roadmap).

Amazon Web Services does not currently provide support for modified copies of this software.

## Security disclosures
If you think you’ve found a potential security issue, please do not post it in the Issues.  Instead, please follow the instructions [here](https://aws.amazon.com/security/vulnerability-reporting/) or [email AWS security directly](mailto:aws-security@amazon.com).

## License

The Amazon ECS Container Agent is licensed under the Apache 2.0 License.
