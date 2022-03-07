# Changelog

## 1.60.0-1
* Cache Agent version 1.60.0
* Add volume plugin to rpm/deb package

## 1.59.0-1
* Cache Agent version 1.59.0
* Log what pre-start is doing

## 1.58.0-2
* Cache Agent version 1.58.0
* Add exec prerequisites to ecs-anywhere installation script

## 1.57.1-1
* Cache Agent version 1.57.1
* Enable AL2 support for ECS-A
* Initialize docker client lazily

## 1.57.0-1
* Cache Agent version 1.57.0

## 1.56.0-1
* Cache Agent version 1.56.0

## 1.55.5-1
* Cache Agent version 1.55.5

## 1.55.4-1
* Cache Agent version 1.55.4
* GPU updates for ECS Anywhere
* Introduce new configuration variable ECS\_OFFHOST\_INTROSPECTION\_NAME to specify the primary network interface name to block offhost agent introspection port access.

## 1.55.3-1
* Cache Agent version 1.55.3

## 1.55.2-1
* Cache Agent version 1.55.2

## 1.55.1-1
* Cache Agent version 1.55.1

## 1.55.0-1
* Cache Agent version 1.55.0

## 1.54.1-1
* Cache Agent version 1.54.1

## 1.54.0-1
* Cache Agent version 1.54.0

## 1.53.1-1
* Cache Agent version 1.53.1

## 1.53.0-1
* Cache Agent version 1.53.0

## 1.52.2-2
* Cache Agent version 1.52.2
* ecs-anywhere-install: fix incorrect download url when running in cn region

## 1.52.2-1
* Cache Agent version 1.52.2
* ecs-anywhere-install: remove dependency on gpg key server
* ecs-anywhere-install: allow sandboxed apt installations

## 1.52.1-1
* Cache Agent version 1.52.1

## 1.52.0-1
* Cache Agent version 1.52.0
* Add support for ECS EXTERNAL launch type (ECS Anywhere)

## 1.51.0-1
* Cache Agent version 1.51.0

## 1.50.3-1
* Cache Agent version 1.50.3

## 1.50.2-1
* Cache Agent version 1.50.2

## 1.50.1-1
* Cache Agent version 1.50.1
* Does not restart ECS Agent when it exits with exit code 5

## 1.50.0-1
* Cache Agent version 1.50.0
* Allows ECS customers to execute interactive commands inside containers.

## 1.49.0-1
* Cache Agent version 1.49.0
* Removes iptable rule that drops packets to port 51678 unconditionally on ecs service stop

## 1.48.1-1
* Cache Agent version 1.48.1

## 1.48.0-2
* Cache Agent version 1.48.0

## 1.47.0-1
* Cache Agent version 1.47.0

## 1.46.0-1
* Cache Agent version 1.46.0

## 1.45.0-1
* Cache Agent version 1.45.0
* Block offhost access to agent's introspection port by default. Configurable via env ECS\_ALLOW\_OFFHOST\_INTROSPECTION\_ACCESS

## 1.44.4-1
* Cache Agent version 1.44.4

## 1.44.3-1
* Cache Agent version 1.44.3

## 1.44.2-1
* Cache Agent version 1.44.2

## 1.44.1-1
* Cache Agent version 1.44.1

## 1.44.0-1
* Cache Agent version 1.44.0
* Add support for configuring Agent container logs

## 1.43.0-2
* Cache Agent version 1.43.0

## 1.42.0-1
* Cache Agent version 1.42.0
* Add a flag ECS\_SKIP\_LOCALHOST\_TRAFFIC\_FILTER to allow skipping local traffic filtering

## 1.41.1-2
* Drop traffic to 127.0.0.1 that isn't originated from the host

## 1.41.1-1
* Cache Agent version 1.41.1

## 1.41.0-1
* Cache Agent version 1.41.0

## 1.40.0-1
* Cache Agent version 1.40.0

## 1.39.0-2
* Cache Agent version 1.39.0
* Ignore IPv6 disable failure if already disabled

## 1.38.0-1
* Cache Agent version 1.38.0
* Adds support for ECS volume plugin
* Disable ipv6 router advertisements for optimization

## 1.37.0-2
* Cache Agent version 1.37.0
* Add '/etc/alternatives' to mounts

## 1.36.2-1
* Cache Agent version 1.36.2
* update sbin mount point to avoid conflict with Docker >= 19.03.5

## 1.36.1-1
* Cache Agent version 1.36.1

## 1.36.0-1
* Cache Agent version 1.36.0
* capture a fixed tail of container logs when removing a container

## 1.35.0-1
* Cache Agent version 1.35.0
* Fix bug where stopping agent gracefully would still restart ecs-init

## 1.33.0-1
* Cache Agent version 1.33.0
* Fix destination path in docker socket bind mount to match the one specified using DOCKER\_HOST on Amazon Linux 2

## 1.32.1-1
* Cache Agent version 1.32.1
* Add the ability to set Agent container's labels

## 1.32.0-1
* Cache Agent version 1.32.0

## 1.31.0-1
* Cache Agent version 1.31.0

## 1.30.0-1
* Cache Agent version 1.30.0

## 1.29.1-1
* Cache Agent version 1.29.1

## 1.29.0-1
* Cache Agent version 1.29.0

## 1.28.1-2
* Cache Agent version 1.28.1
* Use exponential backoff when restarting agent

## 1.28.0-1
* Cache Agent version 1.28.0

## 1.27.0-1
* Cache Agent version 1.27.0

## 1.26.1-1
* Cache Agent version 1.26.1

## 1.26.0-1
* Cache Agent version 1.26.0
* Add support for running iptables within agent container

## 1.25.3-1
* Cache Agent version 1.25.3

## 1.25.2-1
* Cache Agent version 1.25.2

## 1.25.1-1
* Cache Agent version 1.25.1
* Update ecr models for private link support

## 1.25.0-1
* Cache Agent version 1.25.0
* Add Nvidia GPU support for p2 and p3 instances

## 1.24.0-1
* Cache Agent version 1.24.0

## 1.22.0-4
* Cache ECS agent version 1.22.0 for x86\_64 & ARM
* Support ARM architecture builds

## 1.22.0-3
* Rebuild

## 1.22.0-2
* Cache Agent version 1.22.0

## 1.21.0-1
* Cache Agent version 1.21.0
* Support configurable logconfig for Agent container to reduce disk usage
* ECS Agent will use the host's cert store on the Amazon Linux platform

## 1.20.3-1
* Cache Agent version 1.20.3

## 1.20.2-1
* Cache Agent version 1.20.2

## 1.20.1-1
* Cache Agent version 1.20.1

## 1.20.0-1
* Cache Agent version 1.20.0

## 1.19.1-1
* Cache Agent version 1.19.1

## 1.19.0-1
* Cache Agent version 1.19.0

## 1.18.0-2
* Spec file cleanups
* Enable builds for both AL1 and AL2

## 1.18.0-1
* Cache Agent version 1.18.0
* Add support for regional buckets
* Bundle ECS Agent tarball in package
* Download agent based on the partition
* Mount Docker plugin files dir

## 1.17.3-1
* Cache Agent version 1.17.3
* Use s3client instead of httpclient when downloading

## 1.17.2-1
* Cache Agent version 1.17.2

## 1.17.1-1
* Cache Agent version 1.17.1

## 1.17.0-2
* Cache Agent version 1.17.0

## 1.16.2-1
* Cache Agent version 1.16.2
* Add GovCloud endpoint

## 1.16.1-1
* Cache Agent version 1.16.1
* Improve startup behavior when Docker socket doesn't exist yet

## 1.16.0-1
* Cache Agent version 1.16.0

## 1.15.2-1
* Cache Agent version 1.15.2

## 1.15.1-1
* Update ECS Agent version

## 1.15.0-1
* Update ECS Agent version

## 1.14.5-1
* Update ECS Agent version

## 1.14.4-1
* Update ECS Agent version

## 1.14.2-2
* Cache Agent version 1.14.2
* Add functionality for running agent with userns=host when Docker has userns-remap enabled
* Add support for Docker 17.03.1ce

## 1.14.1-1
* Cache Agent version 1.14.1

## 1.14.0-2
* Add retry-backoff for pinging the Docker socket when creating the Docker client

## 1.14.0-1
* Cache Agent version 1.14.0

## 1.13.1-2
* Update Requires to indicate support for docker <= 1.12.6

## 1.13.1-1
* Cache Agent version 1.13.1

## 1.13.0-1
* Cache Agent version 1.13.0

## 1.12.2-1
* Cache Agent version 1.12.2

## 1.12.1-1
* Cache Agent version 1.12.1

## 1.12.0-1
* Cache Agent version 1.12.0
* Add netfilter rules to support host network reaching credentials proxy

## 1.11.1-1
* Cache Agent version 1.11.1

## 1.11.0-1
* Cache Agent version 1.11.0
* Add support for Docker 1.11.2
* Modify iptables and netfilter to support credentials proxy
* Eliminate requirement that /tmp and /var/cache be on the same filesystem
* Start agent with host network mode

## 1.10.0-1
* Cache Agent version 1.10.0
* Add support for Docker 1.11.1

## 1.9.0-1
* Cache Agent version 1.9.0
* Make awslogs driver available by default

## 1.8.2-1
* Cache Agent version 1.8.2

## 1.8.1-1
* Cache Agent version 1.8.1

## 1.8.0-1
* Cache Agent version 1.8.0

## 1.7.1-1
* Cache Agent version 1.7.1

## 1.7.0-1
* Cache Agent version 1.7.0
* Add support for Docker 1.9.1

## 1.6.0-1
* Cache Agent version 1.6.0
* Updated source dependencies

## 1.5.0-1
* Cache Agent version 1.5.0
* Improved merge strategy for user-supplied environment variables
* Add default supported logging drivers

## 1.4.0-2
* Add support for Docker 1.7.1

## 1.4.0-1
* Cache Agent version 1.4.0

## 1.3.1-1
* Cache Agent version 1.3.1
* Read Docker endpoint from environment variable DOCKER\_HOST if present

## 1.3.0-1
* Cache Agent version 1.3.0

## 1.2.1-2
* Cache Agent version 1.2.1

## 1.2.0-1
* Update versioning scheme to match Agent version
* Cache Agent version 1.2.0
* Mount cgroup and execdriver directories for Telemetry feature

## 1.0-5
* Add support for Docker 1.6.2

## 1.0-4
* Properly restart if the ecs-init package is upgraded in isolation

## 1.0-3
* Restart on upgrade if already running

## 1.0-2
* Cache Agent version 1.1.0
* Add support for Docker 1.6.0
* Force cache load on install/upgrade
* Add man page

## 1.0-1
* Re-start Agent on non-terminal exit codes
* Enable Agent self-updates
* Cache Agent version 1.0.0
* Added rollback to cached Agent version for failed updates

## 0.3-0
* Migrate to statically-compiled Go binary

## 0.2-3
* Test for existing container agent and force remove it

## 0.2-2
* Mount data directory for state persistence
* Enable JSON-based configuration

## 0.2-1
* Naive update functionality

