# Amazon ECS Container Agent

[![Build Status](https://travis-ci.org/aws/amazon-ecs-agent.svg?branch=master)](https://travis-ci.org/aws/amazon-ecs-agent)

The Amazon ECS Container Agent is software developed for the [Amazon EC2 Container Service](http://aws.amazon.com/ecs/).

It runs on Container Instances and starts containers on behalf of Amazon ECS.

## Basic Usage

### Docker Image

The Amazon ECS Container Agent should be run in a docker container and may be
downloaded from our [Docker Hub
Repository](https://registry.hub.docker.com/u/amazon/amazon-ecs-agent/).
Documentation on running it properly may be found on the Repository page.

tl;dr: *On an Amazon ECS Container Instance*

1. `touch /etc/ecs/ecs.config`
2. `mkdir -p /var/log/ecs`
3. `docker run --name ecs-agent -d -v /var/run/docker.sock:/var/run/docker.sock -v /var/log/ecs:/log -p
127.0.0.1:51678:51678 --env-file /etc/ecs/ecs.config -e ECS_LOGFILE=/log/ecs-agent.log amazon/amazon-ecs-agent`

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
| `AWS_SESSION_TOKEN` |                         | The [Session Token](http://docs.aws.amazon.com/STS/latest/UsingSTS/Welcome.html) used for temporary credentials. | Taken from EC2 Instance Metadata |

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
