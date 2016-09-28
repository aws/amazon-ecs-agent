# Amazon ECS Container Agent

[![Build Status](https://travis-ci.org/aws/amazon-ecs-agent.svg?branch=master)](https://travis-ci.org/aws/amazon-ecs-agent)

The Amazon ECS Container Agent is software developed for Amazon EC2 Container Service ([Amazon ECS](http://aws.amazon.com/ecs/)).

It runs on container instances and starts containers on behalf of Amazon ECS.

## Usage

The best source of information on running this software is the [Amazon ECS documentation](http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ECS_agent.html).

### On the Amazon Linux AMI

On the [Amazon Linux AMI](https://aws.amazon.com/amazon-linux-ami/), we provide an init package which can be used via `sudo yum install ecs-init && sudo start ecs`. This is the recommended way to run it in this environment.

### On Other AMIs

The Amazon ECS Container Agent may also be run in a Docker container on an EC2 instance with a recent Docker version installed. A Docker image is available in our [Docker Hub Repository](https://registry.hub.docker.com/u/amazon/amazon-ecs-agent/).

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
    --restart=on-failure:10 -d \
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

See also the Advanced Usage section below.

## Building and Running from Source

**Running the Amazon ECS Container Agent outside of Amazon EC2 is not supported.**

### Docker Image

The Amazon ECS Container Agent may be built by typing `make` with the [Docker
daemon](https://docs.docker.com/installation/) (v1.5.0) running.

This produces an image tagged `amazon/ecs-container-agent:make` that
you may run as described above.

### Standalone

The Amazon ECS Container Agent may also be run outside of a Docker container as a
go binary. This is not recommended for production, but it can be useful for
development or easier integration with your local Go tools.

The following commands run the agent outside of Docker:

```
make gobuild
./out/amazon-ecs-agent
```

### Make Targets

The following targets are available. Each may be run with `make <target>`.

| Make Target      | Description |
|:-----------------|:------------|
| `release`        | *(Default)* Builds the agent within a Docker container and and packages it into a scratch-based image |
| `gobuild`        | Runs a normal `go build` of the agent and stores the binary in `./out/amazon-ecs-agent` |
| `static`         | Runs `go build` to produce a static binary in `./out/amazon-ecs-agent` |
| `test`           | Runs all tests using `go test` |
| `test-in-docker` | Runs all tests inside a Docker container |
| `clean`          | Removes build artifacts. *Note: this does not remove Docker images* |

## Advanced Usage

The Amazon ECS Container Agent supports a number of configuration options, most of
which should be set through environment variables.

### Environment Variables

The following environment variables are available. All of them are optional.
They are listed in a general order of likelihood that a user may want to
configure them as something other than the defaults.

| Environment Key | Example Value(s)            | Description | Default Value |
|:----------------|:----------------------------|:------------|:--------------|
| `ECS_CLUSTER`       | clusterName             | The cluster this agent should check into. | default |
| `ECS_RESERVED_PORTS` | `[22, 80, 5000, 8080]` | An array of ports that should be marked as unavailable for scheduling on this container instance. | `[22, 2375, 2376, 51678]` |
| `ECS_RESERVED_PORTS_UDP` | `[53, 123]` | An array of UDP ports that should be marked as unavailable for scheduling on this container instance. | `[]` |
| `ECS_ENGINE_AUTH_TYPE`     |  "docker" &#124; "dockercfg" | The type of auth data that is stored in the `ECS_ENGINE_AUTH_DATA` key. | |
| `ECS_ENGINE_AUTH_DATA`     | See the [dockerauth documentation](https://godoc.org/github.com/aws/amazon-ecs-agent/agent/engine/dockerauth) | Docker [auth data](https://godoc.org/github.com/aws/amazon-ecs-agent/agent/engine/dockerauth) formatted as defined by `ECS_ENGINE_AUTH_TYPE`. | |
| `AWS_DEFAULT_REGION` | &lt;us-west-2&gt;&#124;&lt;us-east-1&gt;&#124;&hellip; | The region to be used in API requests as well as to infer the correct backend host. | Taken from Amazon EC2 instance metadata. |
| `AWS_ACCESS_KEY_ID` | AKIDEXAMPLE             | The [access key](http://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html) used by the agent for all calls. | Taken from Amazon EC2 instance metadata. |
| `AWS_SECRET_ACCESS_KEY` | EXAMPLEKEY | The [secret key](http://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html) used by the agent for all calls. | Taken from Amazon EC2 instance metadata. |
| `DOCKER_HOST`   | unix:///var/run/docker.sock | Used to create a connection to the Docker daemon; behaves similarly to this environment variable as used by the Docker client. | unix:///var/run/docker.sock |
| `ECS_LOGLEVEL`  | &lt;crit&gt; &#124; &lt;error&gt; &#124; &lt;warn&gt; &#124; &lt;info&gt; &#124; &lt;debug&gt; | The level of detail that should be logged. | info |
| `ECS_LOGFILE`   | /ecs-agent.log              | The location where logs should be written. Log level is controlled by `ECS_LOGLEVEL`. | blank |
| `ECS_CHECKPOINT`   | &lt;true &#124; false&gt; | Whether to checkpoint state to the DATADIR specified below. | true if `ECS_DATADIR` is explicitly set to a non-empty value; false otherwise |
| `ECS_DATADIR`      |   /data/                  | The container path where state is checkpointed for use across agent restarts. | /data/ |
| `ECS_UPDATES_ENABLED` | &lt;true &#124; false&gt; | Whether to exit for an updater to apply updates when requested. | false |
| `ECS_UPDATE_DOWNLOAD_DIR` | /cache               | Where to place update tarballs within the container. |  |
| `ECS_DISABLE_METRICS`     | &lt;true &#124; false&gt;  | Whether to disable metrics gathering for tasks. | false |
| `AWS_SESSION_TOKEN` |                         | The [session token](http://docs.aws.amazon.com/STS/latest/UsingSTS/Welcome.html) used for temporary credentials. | Taken from Amazon EC2 instance metadata. |
| `ECS_RESERVED_MEMORY` | 32 | Memory, in MB, to reserve for use by things other than containers managed by Amazon ECS. | 0 |
| `ECS_AVAILABLE_LOGGING_DRIVERS` | `["awslogs","fluentd","gelf","json-file","journald","splunk","syslog"]` | Which logging drivers are available on the container instance. | `["json-file"]` |
| `ECS_DISABLE_PRIVILEGED` | `true` | Whether launching privileged containers is disabled on the container instance. | `false` |
| `ECS_SELINUX_CAPABLE` | `true` | Whether SELinux is available on the container instance. | `false` |
| `ECS_APPARMOR_CAPABLE` | `true` | Whether AppArmor is available on the container instance. | `false` |
| `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION` | 10m | Time to wait to delete containers for a stopped task. If set to less than 1 minute, the value is ignored.  | 3h |
| `ECS_CONTAINER_STOP_TIMEOUT` | 10m | Time to wait for the container to exit normally before being forcibly killed. | 30s |
| `ECS_ENABLE_TASK_IAM_ROLE` | `true` | Whether to enable IAM Roles for Tasks on the Container Instance | `false` |
| `ECS_ENABLE_TASK_IAM_ROLE_NETWORK_HOST` | `true` | Whether to enable IAM Roles for Tasks when launched with `host` network mode on the Container Instance | `false` |
| `ECS_DISABLE_IMAGE_CLEANUP` | `true` | Whether to disable automated image cleanup for the ECS Agent. | `false` |
| `ECS_IMAGE_CLEANUP_INTERVAL` | 30m | The time interval between automated image cleanup cycles. If set to less than 10 minutes, the value is ignored. | 30m |
| `ECS_IMAGE_MINIMUM_CLEANUP_AGE` | 30m | The minimum time interval between when an image is pulled and when it can be considered for automated image cleanup. | 1h |
| `ECS_NUM_IMAGES_DELETE_PER_CYCLE` | 5 | The maximum number of images to delete in a single automated image cleanup cycle. If set to less than 1, the value is ignored. | 5 |

### Persistence

When you run the Amazon ECS Container Agent in production, its `datadir` should be persisted
between runs of the Docker container. If this data is not persisted, the agent registers
a new container instance ARN on each launch and is not able to update the state of tasks it previously ran.

### Flags

The agent also supports the following flags:

* `-k` &mdash; The agent will not require valid SSL certificates for the services that it communicates with.
* ` -loglevel` &mdash; Options: `[<crit>|<error>|<warn>|<info>|<debug>]`. The
agent will output on stdout at the given level. This is overridden by the
`ECS_LOGLEVEL` environment variable, if present.


## Contributing

Contributions and feedback are welcome! Proposals and pull requests will be
considered and responded to. For more information, see the
[CONTRIBUTING.md](https://github.com/aws/amazon-ecs-agent/blob/master/CONTRIBUTING.md)
file.

Amazon Web Services does not currently provide support for modified copies of
this software.


## License

The Amazon ECS Container Agent is licensed under the Apache 2.0 License.
