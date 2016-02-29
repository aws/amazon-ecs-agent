# Amazon ECS Container Agent

[![Build Status](https://travis-ci.org/aws/amazon-ecs-agent.svg?branch=master)](https://travis-ci.org/aws/amazon-ecs-agent)

The Amazon ECS Container Agent is software developed for Amazon EC2 Container Service ([Amazon ECS](http://aws.amazon.com/ecs/)).

It runs on Container Instances and starts containers on behalf of Amazon ECS.

## Usage

The best source of information on running this software is the [AWS Documentation](http://docs.aws.amazon.com/AmazonECS/latest/developerguide/ECS_agent.html).

### On the Amazon Linux AMI

On the [Amazon Linux AMI](https://aws.amazon.com/amazon-linux-ami/), we provide an init package which can be used via `sudo yum install ecs-init && sudo start ecs`. This is the recommended way to run it in this environment.

### On Other AMIs

The Amazon ECS Container Agent may also be run in a Docker container on an EC2 Instance with a recent Docker version installed.
A Docker image is available in our [Docker Hub Repository](https://registry.hub.docker.com/u/amazon/amazon-ecs-agent/).

*Note: The below command should work on most AMIs, but the cgroup and execdriver path may differ in some cases*

```bash
$ mkdir -p /var/log/ecs /etc/ecs /var/lib/ecs/data
$ touch /etc/ecs/ecs.config
$ docker run --name ecs-agent \
    --restart on-failure:10 -d \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /var/log/ecs:/log \
    -v /var/lib/ecs/data:/data \
    -v /var/lib/docker:/var/lib/docker \
    -v /sys/fs/cgroup:/sys/fs/cgroup:ro \
    -v /var/run/docker/execdriver/native:/var/lib/docker/execdriver/native:ro \
    -p 127.0.0.1:51678:51678 \
    --env-file /etc/ecs/ecs.config \
    -e ECS_LOGFILE=/log/ecs-agent.log \
    -e ECS_DATADIR=/data/ \
    amazon/amazon-ecs-agent
```

See also the Advanced Usage section below.

## Building and Running from source

**Please note, running the Amazon ECS Container Agent outside of Amazon EC2 is not supported**

### Docker Image

The Amazon ECS Container Agent may be built by simply typing `make` with the [Docker
daemon](https://docs.docker.com/installation/) (v1.5.0) running.

This will produce an image tagged `amazon/ecs-container-agent:make` which
you may run as described above.

### Standalone

The Amazon ECS Container Agent may also be run outside of a docker container as a
go binary. This is not recommended for production, but it can be useful for
development or easier integration with your local Go tools.

The following commands will run it outside of Docker:

```
make gobuild
./out/amazon-ecs-agent
```

### Make Targets

The following targets are available. Each may be run with `make <target>`.

| Make Target      | Description |
|:-----------------|:------------|
| `release`        | *(Default)* `release` builds the agent within a docker container and and packages it into a scratch-based image |
| `gobuild`        | `gobuild` runs a normal `go build` of the agent and stores the binary in `./out/amazon-ecs-agent` |
| `static`         | `static` runs `go build` to produce a static binary in `./out/amazon-ecs-agent` |
| `test`           | `test` runs all tests using `go test` |
| `test-in-docker` | `test-in-docker` runs all tests inside a docker container |
| `clean`          | `clean` removes build artifacts. *Note: this does not remove docker images* |

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
| `ECS_RESERVED_PORTS` | `[22, 80, 5000, 8080]` | An array of ports that should be marked as unavailable for scheduling on this Container Instance. | `[22, 2375, 2376, 51678]` |
| `ECS_RESERVED_PORTS_UDP` | `[53, 123]` | An array of UDP ports that should be marked as unavailable for scheduling on this Container Instance. | `[]` |
| `ECS_ENGINE_AUTH_TYPE`     |  "docker" &#124; "dockercfg" | What type of auth data is stored in the `ECS_ENGINE_AUTH_DATA` key | |
| `ECS_ENGINE_AUTH_DATA`     | See [documentation](https://godoc.org/github.com/aws/amazon-ecs-agent/agent/engine/dockerauth) | Docker [auth data](https://godoc.org/github.com/aws/amazon-ecs-agent/agent/engine/dockerauth) formatted as defined by `ECS_ENGINE_AUTH_TYPE`. | |
| `AWS_DEFAULT_REGION` | &lt;us-west-2&gt;&#124;&lt;us-east-1&gt;&#124;&hellip; | The region to be used in API requests as well as to infer the correct backend host. | Taken from EC2 Instance Metadata |
| `AWS_ACCESS_KEY_ID` | AKIDEXAMPLE             | The [Access Key](http://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html) used by the agent for all calls. | Taken from EC2 Instance Metadata |
| `AWS_SECRET_ACCESS_KEY` | EXAMPLEKEY | The [Secret Key](http://docs.aws.amazon.com/general/latest/gr/aws-security-credentials.html) used by the agent for all calls. | Taken from EC2 Instance Metadata |
| `DOCKER_HOST`   | unix:///var/run/docker.sock | Used to create a connection to the Docker daemon; behaves similarly to this environment variable as used by the Docker client. | unix:///var/run/docker.sock |
| `ECS_LOGLEVEL`  | &lt;crit&gt; &#124; &lt;error&gt; &#124; &lt;warn&gt; &#124; &lt;info&gt; &#124; &lt;debug&gt; | What level to log at on stdout. | info |
| `ECS_LOGFILE`   | /ecs-agent.log              | The path to output full debugging info to. If blank, no logs will be written to file. If set, logs at debug level (regardless of ECS\_LOGLEVEL) will be written to that file. | blank |
| `ECS_CHECKPOINT`   | &lt;true &#124; false&gt; | Whether to checkpoint state to the DATADIR specified below | true if `ECS_DATADIR` is explicitly set to a non-empty value; false otherwise |
| `ECS_DATADIR`      |   /data/                  | The container path where state is checkpointed for use across agent restarts. | /data/ |
| `ECS_UPDATES_ENABLED` | &lt;true &#124; false&gt; | Whether to exit for an updater to apply updates when requested | false |
| `ECS_UPDATE_DOWNLOAD_DIR` | /cache               | Where to place update tarballs within the container |  |
| `ECS_DISABLE_METRICS`     | &lt;true &#124; false&gt;  | Whether to disable metrics gathering for tasks. | false |
| `ECS_DOCKER_GRAPHPATH`   | /var/lib/docker | Used to create the path to the state file of containers launched. The state file is used to read utilization metrics of containers. | /var/lib/docker |
| `AWS_SESSION_TOKEN` |                         | The [Session Token](http://docs.aws.amazon.com/STS/latest/UsingSTS/Welcome.html) used for temporary credentials. | Taken from EC2 Instance Metadata |
| `ECS_RESERVED_MEMORY` | 32 | Memory, in MB, to reserve for use by things other than containers managed by ECS. | 0 |
| `ECS_AVAILABLE_LOGGING_DRIVERS` | `["json-file","syslog"]` | Which logging drivers are available on the Container Instance. | `["json-file"]` |
| `ECS_DISABLE_PRIVILEGED` | `true` | Whether launching privileged containers is disabled on the Container Instance. | `false` |
| `ECS_SELINUX_CAPABLE` | `true` | Whether SELinux is available on the Container Instance. | `false` |
| `ECS_APPARMOR_CAPABLE` | `true` | Whether AppArmor is available on the Container Instance. | `false` |
| `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION` | 10m | Time to wait to delete containers for a stopped task. If set to less than 1 minute, the value will be ignored.  | 3h |

### Persistence

When running the Amazon ECS Container Agent in production, its `datadir` should be persisted
between runs of the Docker container. If this data is not persisted, the Amazon ECS Agent will register
a new Container Instance ARN on each launch and will not be able to update the state of tasks it previously ran.

### Flags

The agent also supports the following flags:

* `-k` &mdash; The agent will not requre valid SSL certificates for the services it communicates with.
* ` -loglevel` &mdash; Options: `[<crit>|<error>|<warn>|<info>|<debug>]`. The
agent will output on stdout at the given level. This is overridden by the
`ECS_LOGLEVEL` environment variable, if present.


## Contributing

Contributions and feedback are welcome! Proposals and Pull Requests will be
considered and responded to. Please see the
[CONTRIBUTING.md](https://github.com/aws/amazon-ecs-agent/blob/master/CONTRIBUTING.md)
file for more information.

Amazon Web Services does not currently provide support for modified copies of
this software.


## License

The Amazon ECS Container Agent is licensed under the Apache 2.0 License.
