# Changelog
## 1.55.1
* Enhancement - Third party dependency version updates [#2982](https://github.com/aws/amazon-ecs-agent/pull/2982) [#2983](https://github.com/aws/amazon-ecs-agent/pull/2983)

## 1.55.0
* Feature - Support buffer limit option in FireLens [#2958](https://github.com/aws/amazon-ecs-agent/pull/2958)
* Enhancement - Introduce optional jitter for task cleanup wait duration, configurable via `ECS_ENGINE_TASK_CLEANUP_WAIT_DURATION_JITTER` environment variable. In use case where there are large number of tasks being stopped at the same time, specifying this jitter can help avoid all the task cleanup happening at the same time (the latter could add pressure to the instance and as a result affect running tasks) [#2969](https://github.com/aws/amazon-ecs-agent/pull/2969)

## 1.54.1
* Enhancement - Get container's exit code from docker event in case we receive a container die event, but fail to inspect the container. Previously the container's exit code was left as null in this case. [#2940](https://github.com/aws/amazon-ecs-agent/pull/2940)

## 1.54.0
* Feature - ECS EC2 task networking for Windows tasks [#2915](https://github.com/aws/amazon-ecs-agent/pull/2915)
* Bug - Upgrading the amazon-vpc-cni plugins submodule to address a bug on Windows Server 2004 and Windows Server 20H2 platforms [#2930](https://github.com/aws/amazon-ecs-agent/pull/2930)

## 1.53.1
* Bug - Fix broken DataDir/Checkpoint file configuration [#2897](https://github.com/aws/amazon-ecs-agent/pull/2897)
* Enhancement - Update Docker Library to v19.03.11 [#2905](https://github.com/aws/amazon-ecs-agent/pull/2905)

## 1.53.0
* Bug - Revert change that registered Windows ECS Instances using specific OSFamilyType [#2859](https://github.com/aws/amazon-ecs-agent/pull/2859) to address [#2881](https://github.com/aws/amazon-ecs-agent/issues/2881)
* Bug - Fix an edge case that could incorrectly mark a task as STOPPED when Docker crashes while stopping a container [#2885](https://github.com/aws/amazon-ecs-agent/pull/2885)

## 1.52.2
* Enhancement - validate agent config file path permission on Windows [#2866](https://github.com/aws/amazon-ecs-agent/pull/2866)
* Bug - fix potential goroutine leak when closing websocket connections [#2854](https://github.com/aws/amazon-ecs-agent/pull/2854)
* Bug - fixes a bug where a task can be stuck in RUNNING indefinitely when a container can't be stopped due to an unresolved docker [bug](https://github.com/moby/moby/issues/41587) (see also the open [PR](https://github.com/moby/moby/pull/41588) in moby to fix the bug).

## 1.52.1
* Enhancement - Register Windows ECS Instances using specific OSFamilyType [#2859](https://github.com/aws/amazon-ecs-agent/pull/2859)
* Enhancement - Add retries while retrieving instance-id using EC2 Instance metadata service api [#2861](https://github.com/aws/amazon-ecs-agent/pull/2861)

## 1.52.0
* Enhancement - Support for ECS EXTERNAL launch type (ECS Anywhere) [#2849](https://github.com/aws/amazon-ecs-agent/pull/2849)
* Enhancement - Add support for ECS agent to acknowledge server heartbeat messages [#2837](https://github.com/aws/amazon-ecs-agent/pull/2837)

## 1.51.0
* Enhancement - Add configurable agent healthcheck localhost ip env var. [#2834](https://github.com/aws/amazon-ecs-agent/pull/2834)
* Bug - Fix bug that could incorrectly clean up pause container before other containers. [#2838](https://github.com/aws/amazon-ecs-agent/pull/2838)
* Bug - Fix task's network stats by omitting pause container in the network metrics calculation. [#2836](https://github.com/aws/amazon-ecs-agent/pull/2836)

## 1.50.3
* Enhancement - Eliminate benign docker stats "context canceled" warning messages from logs [#2813](https://github.com/aws/amazon-ecs-agent/pull/2813)
* Bug - Fix bug where pause container was not always cleaned up [#2824](https://github.com/aws/amazon-ecs-agent/pull/2824)

## 1.50.2
* Bug - Fix potential deadlock due to seelog's string marshalling of task struct [#2811](https://github.com/aws/amazon-ecs-agent/pull/2811)

## 1.50.1
* Enhancement - Implementation of structured logs on top of seelog [#2797](https://github.com/aws/amazon-ecs-agent/pull/2797)
* Bug - Fixed a task status deadlock and pulled container state for cached images when ECS_PULL_DEPENDENT_CONTAINERS_UPFRONT is enabled [#2800](https://github.com/aws/amazon-ecs-agent/pull/2800)

## 1.50.0
* Feature - Allows ECS customers to execute interactive commands inside containers [#2798](https://github.com/aws/amazon-ecs-agent/pull/2798)
* Enhancement - Add error responses into TMDEv4 taskWithTags responses [#2789](https://github.com/aws/amazon-ecs-agent/pull/2789)
* Bug - Fixed the number of cpu units the Agent will reserve for the Linux container instances [#2783](https://github.com/aws/amazon-ecs-agent/pull/2783)

## 1.49.0
* Enhancement - Allow task metadata endpoint to return metadata for task when some of the container does not have network metadata [#2747](https://github.com/aws/amazon-ecs-agent/pull/2747)
* Enhancement - Improve error and info logging around credentials requests [#2705](https://github.com/aws/amazon-ecs-agent/pull/2705)
* Enhancement - Introduce new environment variable ECS_CONTAINER_CREATE_TIMEOUT to make Docker create timeout configurable. Minimum value is 1m. Default value is 4m. [#2781](https://github.com/aws/amazon-ecs-agent/pull/2781)
* Bug - Add missing error handling in getContainerStatsNotStreamed. [#2757](https://github.com/aws/amazon-ecs-agent/pull/2757)

## 1.48.1
* Bug - Fix an edge case that can cause container dependency deadlock [#2734](https://github.com/aws/amazon-ecs-agent/pull/2734)
* Bug - Revert the change that adds client token persistence [#2708](https://github.com/aws/amazon-ecs-agent/pull/2708)

## 1.48.0
* Enhancement - Docker stop timeout buffer increased from 30s to 2m [#2697](https://github.com/aws/amazon-ecs-agent/pull/2697)
* Enhancement - More informative ENI attachment logs [#2703](https://github.com/aws/amazon-ecs-agent/pull/2703)
* Enhancement - Introduce new environment variable ECS_PULL_DEPENDENT_CONTAINERS_UPFRONT to pull images of dependent containers even before dependsOn condition is satisfied. This feature is turned off by default [#2731](https://github.com/aws/amazon-ecs-agent/pull/2731)
* Enhancement - Add pulled containers metadata to Task Metadata Endpoint V4 [#2731](https://github.com/aws/amazon-ecs-agent/pull/2731)
* Bug - Fix a bug where agent persists RCI client token to avoid being registered as different container instance ARNs [#2708](https://github.com/aws/amazon-ecs-agent/pull/2708)
* Bug - Fix jumbled min & max for engine connection retry delays [#2721](https://github.com/aws/amazon-ecs-agent/pull/2721)

## 1.47.0
* Feature - Add support for FSxWindowsFileServerVolumeConfiguration in task definition [#2690](https://github.com/aws/amazon-ecs-agent/pull/2690)
* Bug - Fixed Makefile to use Go1.12 for Agent windows build
[#2688](https://github.com/aws/amazon-ecs-agent/pull/2688)
* Bug - Initialize the logger from the agent’s main() [#2644](https://github.com/aws/amazon-ecs-agent/pull/2644)

## 1.46.0
* Enhancement -  Use Go 1.15 for Linux platforms and Go 1.12 for Windows platforms [#2653](https://github.com/aws/amazon-ecs-agent/pull/2653)
* Bug - Currently, while polling docker stats, there is no timeout for the API call. So the call could be stuck until the container is stopped. Adding poll stats timeout [#2656](https://github.com/aws/amazon-ecs-agent/pull/2656)

## 1.45.0
* Feature - ECS metadata for AWS service lens and x-ray. We have added new fields to the TMDEv4 endpoint. Specfically, ContainerARN, LogDriver, LogOptions, and LaunchType [#2623](https://github.com/aws/amazon-ecs-agent/pull/2623)
* Feature - add IPv6 support for task networking [#2646](https://github.com/aws/amazon-ecs-agent/pull/2646)
* Enhancement - Propagate responses to the Amazon ECS Container Agent Log when an error occurs [#2641](https://github.com/aws/amazon-ecs-agent/pull/2641)
* Bug - Fix HTTP response code to TMDE requests when they fail for internal reasons. Previously we returned 400 Bad Request, and will now return 500 Internal Server Error [#2643](https://github.com/aws/amazon-ecs-agent/pull/2643)

## 1.44.4
* Bug - Fix a bug where the ECS Agent did not iterate through all the dependencies of a particular container [#2615](https://github.com/aws/amazon-ecs-agent/pull/2615)
* Bug - Fix a bug where the ECS Agent can lose track of containers if it's stopped by SIGKILL instead of SIGTERM [#2609](https://github.com/aws/amazon-ecs-agent/pull/2609)
* Bug - Fix a bug where the a Docker API call could be made with a blank string instead of a Docker ID [#2608](https://github.com/aws/amazon-ecs-agent/pull/2608)
* Bug - Fix a bug where the ECS Agent was expecting ECS_LOGFILE to be present as an environment variable [#2598](https://github.com/aws/amazon-ecs-agent/pull/2598)

## 1.44.3
* Bug - Revert Introspection API scope change [#2605](https://github.com/aws/amazon-ecs-agent/pull/2605)
* Bug - Fix a bug where ECS_LOGLEVEL stopped controlling logging level on instance [#2597](https://github.com/aws/amazon-ecs-agent/pull/2597) 

## 1.44.2
* Bug - Fix Introspection API scope and bind to localhost [#2588](https://github.com/aws/amazon-ecs-agent/pull/2588)
* Enhancement - Make image pull timeout configurable [#2565](https://github.com/aws/amazon-ecs-agent/pull/2565)


## 1.44.1
* Bug - Fixes a bug where ENI is attached before Agent starts and there is a delay in acknowledgement of ENI attachment by Agent [#2581](https://github.com/aws/amazon-ecs-agent/pull/2581)
* Bug - Fixes a deadlock scenario when the agent restores the state from its data file and the tasks are using environment files feature [#2580](https://github.com/aws/amazon-ecs-agent/pull/2580)
* Bug - Fixed a bug that can cause stats endpoint to return empty response to container that just starts up [#2578](https://github.com/aws/amazon-ecs-agent/pull/2578)
* Bug - Fixed a bug where parsing logic from env var parsing was introducing error depending on the value [#2573](https://github.com/aws/amazon-ecs-agent/pull/2573)

## 1.44.0
* Feature - Add support for customers to configure the destination for the Agent container logs, by setting a Docker-supported logging driver in the Agent config file - [#2548](https://github.com/aws/amazon-ecs-agent/pull/2548)
* Enhancement - Agent's internal state management mechanism is changed from a custom json state file to boltdb. This change is made to reduce its resource consumption especially under high task density/mutation rate - [#2562](https://github.com/aws/amazon-ecs-agent/pull/2562)

## 1.43.0
* Feature - Collect network stats for awsvpc network mode and display network rate stats for bridge and awsvpc network mode through v4 metadata endpoint - [#2545](https://github.com/aws/amazon-ecs-agent/pull/2545)

## 1.42.0
* Feature - Support for sub second precision in FluentD [#2538](https://github.com/aws/amazon-ecs-agent/pull/2538).
* Bug - Fixed a bug that caused configured values for ImageCleanupExclusionList
to be ignored in some situations [#2513](https://github.com/aws/amazon-ecs-agent/pull/2513)

## 1.41.1
* Bug - Fixed a bug [#2476](https://github.com/aws/amazon-ecs-agent/issues/2476) where HostPort is not present in ECS Task Metadata Endpoint response with bridge network type [#2495](https://github.com/aws/amazon-ecs-agent/pull/2495)

## 1.41.0
* Feature - Add inferentia support [#2458](https://github.com/aws/amazon-ecs-agent/pull/2458)
* Bug - fixes a bug where env file feature would not accept "=", which is the delimiter in the values of a env var [#2487](https://github.com/aws/amazon-ecs-agent/pull/2487)

## 1.40.0
* Enhancement - Agent's default stats gathering is changing from docker streaming stats to polling. This should not affect the metrics that customers ultimately see in cloudwatch, but it does affect how the agent gathers the underlying metrics from docker. This change was made for considerable performance gains. Customers with high CPU loads may see their cluster utilization increase; this is a good thing because it means the containers are utilizing more of the cluster, and agent/dockerd/containerd are utilizing less [#2452](https://github.com/aws/amazon-ecs-agent/pull/2452)
* Enhancement - Adds a jitter to this so that we don't query docker for every container's state all at the same time [#2444](https://github.com/aws/amazon-ecs-agent/pull/2444)
* Bug - Register custom logger before it gets used to ensure that the formatter is initiated before it is loaded [#2438](https://github.com/aws/amazon-ecs-agent/pull/2438)

## 1.39.0
* Feature - Add support for bulk loading env vars through environmentFiles field in task definition [#2420](https://github.com/aws/amazon-ecs-agent/pull/2420)
* Feature - Add v4 task metadata endpoint, which includes additional network information compared to v3 [#2396](https://github.com/aws/amazon-ecs-agent/pull/2396)
* Bug - Fixed an edge case that can cause task failed to start when using container ordering success condition [#2404](https://github.com/aws/amazon-ecs-agent/pull/2404).

## 1.38.0
* Feature - add integration with EFS's access point and IAM authorization features; support EFS volume for task in awsvpc network mode
* Enhancement - adding Runtime ID of container to agent logs [#2399](https://github.com/aws/amazon-ecs-agent/pull/2399)

## 1.37.0
* Feature - additional parameters allowed when specifying secretsmanager secret [#2358](https://github.com/aws/amazon-ecs-agent/pull/2358)
* Bug - fixed a bug where Firelens container could not use config file from S3 bucket in us-east-1 [#2356](https://github.com/aws/amazon-ecs-agent/pull/2356)

## 1.36.2
* Bug - fix windows logfile writing [#2347](https://github.com/aws/amazon-ecs-agent/pull/2347)
* Bug - update sbin mount point to avoid conflict with Docker >= 19.03.5 [#2345](https://github.com/aws/amazon-ecs-agent/pull/2345)

## 1.36.1
* Bug - Fixed potential file descriptor leak with context logger [#2337](https://github.com/aws/amazon-ecs-agent/pull/2337)
 
## 1.36.0
* Feature - structured logs and logfile rollover features [#2311](https://github.com/aws/amazon-ecs-agent/pull/2311), [#2319](https://github.com/aws/amazon-ecs-agent/pull/2319), [#2330](https://github.com/aws/amazon-ecs-agent/pull/2330)

## 1.35.0
* Feature - EFS Preview [#2301](https://github.com/aws/amazon-ecs-agent/pull/2301)
* Bug - Load pause container for use by PID/IPC even if task networking is disabled [#2300](https://github.com/aws/amazon-ecs-agent/pull/2300)
* Bug - Fixed a race condition that might cause the agent to crash during container creation [#2299](https://github.com/aws/amazon-ecs-agent/pull/2299)

## 1.34.0
* Feature - Add Windows gMSA (group Managed Service Account) support on ECS.
* Bug - Binding metadata directory in Z mode for selinux enabled docker, which enables read access to the metadata files from container processes. [#2273](https://github.com/aws/amazon-ecs-agent/pull/2273)

## 1.33.0
* Feature - Agent performs a sync between task state on the instance and on the backend everytime Agent establishes a connection with the backend. This ensures that task state is as expected on the instance after the instance reconnects with the instance after a disconnection [#2191](https://github.com/aws/amazon-ecs-agent/pull/2191)
* Enhancement - Update Docker LoadImage API timeout based on benchmarking test [#2269](https://github.com/aws/amazon-ecs-agent/pull/2269)
* Enhancement - Enable the detection of health status of ECS Agent using HEALTHCHECK directive [#2260](https://github.com/aws/amazon-ecs-agent/pull/2260)  
* Enhancement - Add `NON_ECS_IMAGE_MINIMUM_CLEANUP_AGE` flag which when set allows the user to set the minimum time interval between when a non ECS image is created and when it can be considered for automated image cleanup [#2251](https://github.com/aws/amazon-ecs-agent/pull/2251)

## 1.32.1
* Enhancement - Add `ECS_ENABLE_MEMORY_UNBOUNDED_WINDOWS_WORKAROUND` flag which when set ignores the memory reservation 
parameter along with memory bounded tasks in windows [@julienduchesne](https://github.com/julienduchesn) [#2239](https://github.com/aws/amazon-ecs-agent/pull/2239)
* Bug - Fixed a bug when config attribute in hostConfig is nil when starting a task [#2249](https://github.com/aws/amazon-ecs-agent/pull/2249)
* Bug - Fixed a bug where start container failed with EOF and container is started anyways [#2245](https://github.com/aws/amazon-ecs-agent/pull/2245) 
* Bug - Fixed a bug where incorrect error type was detected when the `vpc-id` could not be detected on ec2-classic [#2243](https://github.com/aws/amazon-ecs-agent/pull/2243)
* Bug - Fixed a bug where Agent did not reopen Docker event stream when it gets EOF/UnexpectedEOF error [#2240](https://github.com/aws/amazon-ecs-agent/pull/2240)

## 1.32.0
* Feature - Add support for automatic spot instance draining [#2205](https://github.com/aws/amazon-ecs-agent/pull/2205)
* Bug - Fixed a bug where the agent might crash if it's restarted right after launching a task in awsvpc network mode [#2219](https://github.com/aws/amazon-ecs-agent/pull/2219)

## 1.31.0
* Feature - Add support for showing container's ImageDigest Pulled from ECR in ECS DescribeTasks [#2201](https://github.com/aws/amazon-ecs-agent/pull/2201)
* Enhancement - Add more functionalities to firelens (log router) feature: allow including external config from s3 and local file; add fluent logger support for bridge and awsvpc network mode; add health check support for aws-for-fluent-bit image
* Enhancement - Add support for Windows Named Pipes in volumes [@ericdalling](https://github.com/ericdalling) [#2185](https://github.com/aws/amazon-ecs-agent/pull/2185)

## 1.30.0
* Feature - Add log router support (beta)
* Feature - Add elastic inference support
* Feature - Add support for showing container's Docker ID in ECS DescribeTasks and StopTask APIs [#2138](https://github.com/aws/amazon-ecs-agent/pull/2138)

## 1.29.1
* Enhancement - Update task cleanup wait logic to clean task resources immediately instead of waiting 3 hours [#2084](https://github.com/aws/amazon-ecs-agent/pull/2084)
* Bug - Fixed Agent reporting incorrect capabilities on Windows [#2070](https://github.com/aws/amazon-ecs-agent/pull/2070)
* Bug - Fixed a bug where Agent fails to invoke IPAM DEL command when cleaning up AWSVPC task [#2085](https://github.com/aws/amazon-ecs-agent/pull/2085)
* Bug - Fixed a bug where task resource unmarshal error was ignored rather than returned [#2098](https://github.com/aws/amazon-ecs-agent/pull/2098)
* Bug - Update amazon-vpc-plugins that allows AWSVPCTrunking to work without ec2-net-utils [#2093](https://github.com/aws/amazon-ecs-agent/pull/2093)

## 1.29.0
* Feature - Adds container network and storage metrics as part of ongoing [work](https://github.com/aws/containers-roadmap/issues/70) [#2072](https://github.com/aws/amazon-ecs-agent/pull/2072)

## 1.28.1
* Enhancement - Non-ECS images cleanup: clean up dangling images with image ID [#2023](https://github.com/aws/amazon-ecs-agent/pull/2023)
* Bug - Pick up latest version of amazon-vpc-cni-plugins and amazon-ecs-cni-plugins to include recent bug fixes ([f09fd7c](https://github.com/aws/amazon-vpc-cni-plugins/commit/f09fd7c6ba0cf319b0c6ad23762091e25091fbce), [d90eebe](https://github.com/aws/amazon-vpc-cni-plugins/commit/d90eebe9907cde58c47756f65eccd7efc693e1d6), [06cbba2](https://github.com/aws/amazon-ecs-cni-plugins/commit/06cbba25cab0eb0aa466b0c5f72b55b61c87a2c5))
* Bug - Fixed error detection case when image that is being deleted does not exist [@bendavies](https://github.com/bendavies) [#2008](https://github.com/aws/amazon-ecs-agent/pull/2008)
* Bug - Fixed a bug where docker volume deletion resulted in nullpointer [#2059](https://github.com/aws/amazon-ecs-agent/pull/2059)

## 1.28.0
* Feature - Introduce high density awsvpc tasks support
* Enhancement - Introduce `ECS_CGROUP_CPU_PERIOD` to make cgroup cpu period configurable [@boynux](https://github.com/boynux) [#1941](https://github.com/aws/amazon-ecs-agent/pull/1941)
* Enhancement - Add Private Host IPv4 address to container metadata [@bencord0](https://github.com/bencord0) [#2000](https://github.com/aws/amazon-ecs-agent/pull/2000)
* Enhancement - Set terminal reason for volume task resource [#2004](https://github.com/aws/amazon-ecs-agent/pull/2004)
* Bug - Fixed a bug where container health status is not updated when container status isn't changed [#1972](https://github.com/aws/amazon-ecs-agent/pull/1972)
* Bug - Fixed a bug where containers in 'dead' or 'created' status are not cleaned up by the agent [#2015](https://github.com/aws/amazon-ecs-agent/pull/2015)

## 1.27.0
* Feature - Add secret support for log drivers

## 1.26.1
* Enhancement - Set up pause container user the same as proxy container when App Mesh enabled and pause container not using default image
* Bug - Fixed a bug where network stats are not presented in container stats [#1932](https://github.com/aws/amazon-ecs-agent/pull/1932)

## 1.26.0
* Feature - Startup order can now be explicitly set via DependsOn field in the Task Definition [#1904](https://github.com/aws/amazon-ecs-agent/pull/1904)
* Feature - Containers in a task can now have individual start and stop timeouts [#1904](https://github.com/aws/amazon-ecs-agent/pull/1904)
* Feature - AWS App Mesh CNI plugin support [#1898](https://github.com/aws/amazon-ecs-agent/pull/1898)
* Enhancement - Containers with links and volumes defined will now shutdown in the correct order [#1904](https://github.com/aws/amazon-ecs-agent/pull/1904)
* Bug - Image cleanup errors fixed [#1897](https://github.com/aws/amazon-ecs-agent/pull/1897)

## 1.25.3
* Bug - Fixed a bug where agent no longer redirected malformed credentials or metadata http requests [#1844](https://github.com/aws/amazon-ecs-agent/pull/1844)
* Bug - Populate v3 metadata networks response for non-awsvpc tasks [#1833](https://github.com/aws/amazon-ecs-agent/pull/1833)
* Bug - Avoid image pull behavior type check for container namespaces pause image [#1840](https://github.com/aws/amazon-ecs-agent/pull/1840)

## 1.25.2
* Bug - Update pull image retry to longer interval to mitigate being throttled by ECR [#1808](https://github.com/aws/amazon-ecs-agent/pull/1808)

## 1.25.1
* Bug - Update ecr models for private link support

## 1.25.0
* Feature - Add Nvidia GPU support for p2 and p3 instances
* Feature - Introduce `ECS_DISABLE_DOCKER_HEALTH_CHECK` to make docker health check configurable [#1624](https://github.com/aws/amazon-ecs-agent/pull/1624)

## 1.24.0
* Feature - Configurable poll duration for container stats [@jcbowman](https://github.com/jcbowman) [#1646](https://github.com/aws/amazon-ecs-agent/pull/1646)
* Feature - Add support to remove containers and images that are not part of ECS tasks [#1752](https://github.com/aws/amazon-ecs-agent/pull/1752)
* Feature - Introduce prometheus support for agent metrics [#1745](https://github.com/aws/amazon-ecs-agent/pull/1745)
* Feature - Add Host EC2 instance Public IPv4 address to container metadata file [#1730](https://github.com/aws/amazon-ecs-agent/pull/1730)
* Enhancement - Docker SDK migration replacing go-dockerclient [#1743](https://github.com/aws/amazon-ecs-agent/pull/1743)
* Enhancement - Propagating Container Instance and Task Tags to Task Metadata endpoint [#1720](https://github.com/aws/amazon-ecs-agent/pull/1720)

## 1.23.0
* Feature - Add support for ECS Secrets integrating with AWS Secrets Manager [#1713](https://github.com/aws/amazon-ecs-agent/pull/1713)
* Enhancement - Add availability zone to task metadata endpoint [#1674](https://github.com/aws/amazon-ecs-agent/pull/1674)
* Enhancement - Add availability zone to ECS metadata file [#1675](https://github.com/aws/amazon-ecs-agent/pull/1675)
* Bug - Fixed a bug where agent can register container instance back to back and gets
  assigned two container instance ARNs [#1711](https://github.com/aws/amazon-ecs-agent/pull/1711)
* Bug - Fixed a bug where propagated `aws:` tags are passed through RegisterContainerInstance API call [#1706](https://github.com/aws/amazon-ecs-agent/pull/1706)

## 1.22.0
* Feature - Add support for ECS Secrets integrating with AWS Systems Manager Parameter Store
* Feature - Support for `--pid`, `--ipc` Docker run flags. [#1584](https://github.com/aws/amazon-ecs-agent/pull/1584)
* Feature - Introduce two environment variables `ECS_CONTAINER_INSTANCE_PROPAGATE_TAGS_FROM` and `ECS_CONTAINER_INSTANCE_TAGS` to support ECS tagging [#1618](https://github.com/aws/amazon-ecs-agent/pull/1618)

## 1.21.0
* Feature - Add v3 task metadata support for awsvpc, host and bridge network mode
* Enhancement - Update the `amazon-ecs-cni-plugins` to `2018.10.0` [#1610](https://github.com/aws/amazon-ecs-agent/pull/1610)
* Enhancement - Configurable image pull inactivity timeout [@wattdave](https://github.com/wattdave) [#1566](https://github.com/aws/amazon-ecs-agent/pull/1566)
* Bug - Fixed a bug where Windows drive volume couldn't be mounted [#1571](https://github.com/aws/amazon-ecs-agent/pull/1571)
* Bug - Fixed a bug where the Agent's Windows binaries didn't use consistent naming [#1573](https://github.com/aws/amazon-ecs-agent/pull/1573)
* Bug - Fixed a bug where a port used by WinRM service was not reserved by the Agent by default [#1577](https://github.com/aws/amazon-ecs-agent/pull/1577)

## 1.20.3
* Enhancement - Deprecate support for serial docker image pull [#1569](https://github.com/aws/amazon-ecs-agent/pull/1569)
* Enhancement - Update the `amazon-ecs-cni-plugins` to `2018.08.0`

## 1.20.2
* Enhancement - Added ECS config field `ECS_SHARED_VOLUME_MATCH_FULL_CONFIG` to
make the volume labels and driver options comparison configurable for shared volume [#1519](https://github.com/aws/amazon-ecs-agent/pull/1519)
* Enhancement - Added Volumes metadata as part of v1 and v2 metadata endpoints [#1531](https://github.com/aws/amazon-ecs-agent/pull/1531)
* Bug - Fixed a bug where unrecognized task cannot be stopped [#1467](https://github.com/aws/amazon-ecs-agent/pull/1467)
* Bug - Fixed a bug where tasks with CPU windows unbounded field set are not honored
on restart due to non-persistence of `PlatformFields` in agent state file [@julienduchesne](https://github.com/julienduchesne) [#1480](https://github.com/aws/amazon-ecs-agent/pull/1480)

## 1.20.1
* Bug - Fixed a bug where the agent couldn't be upgraded if there are tasks that
  use volumes in the task definition on the instance
* Bug - Fixed a bug where volumes driver may not work with mountpoint

## 1.20.0
* Feature - Add support for Docker volume drivers, third party drivers are only supported on linux
* Enhancement - Replace the empty container with Docker local volume
* Enhancement - Deprecate support for Docker version older than 1.9.0 [#1477](https://github.com/aws/amazon-ecs-agent/pull/1477)
* Bug - Fixed a bug where container marked as stopped comes back with a running status [#1446](https://github.com/aws/amazon-ecs-agent/pull/1446)

## 1.19.1
* Bug - Fixed a bug where responses of introspection API break backward compatibility [#1473](https://github.com/aws/amazon-ecs-agent/pull/1473)

## 1.19.0
* Feature - Private registry can be authenticated through task definition using AWS Secrets Manager [#1427](https://github.com/aws/amazon-ecs-agent/pull/1427)

## 1.18.0
* Feature - Configurable container image pull behavior [#1348](https://github.com/aws/amazon-ecs-agent/pull/1348)
* Bug - Fixed a bug where Docker Version() API never returns by adding a timeout [#1363](https://github.com/aws/amazon-ecs-agent/pull/1363)
* Bug - Fixed a bug where tasks could get stuck waiting for execution of CNI plugin [#1358](https://github.com/aws/amazon-ecs-agent/pull/1358)
* Bug - Fixed a bug where task cleanup could be blocked due to incorrect sentstatus [#1383](https://github.com/aws/amazon-ecs-agent/pull/1383)

## 1.17.3
* Enhancement - Distinct startContainerTimeouts for windows/linux, introduce a new environment variable `ECS_CONTAINER_START_TIMEOUT` to make it configurable [#1321](https://github.com/aws/amazon-ecs-agent/pull/1321)
* Enhancement - Add support for containers to inherit ENI private DNS hostnames for `awsvpc` tasks [#1278](https://github.com/aws/amazon-ecs-agent/pull/1278)
* Enhancement - Expose task definition family and task definition revision in container metadata file [#1295](https://github.com/aws/amazon-ecs-agent/pull/1295)
* Enhancement - Fail image pulls if there's inactivity during image pull progress [#1290](https://github.com/aws/amazon-ecs-agent/pull/1290)
* Enhancement - Parallelize the container transition in the same task [#1305](https://github.com/aws/amazon-ecs-agent/pull/1306)
* Bug - Fixed a bug where a stale websocket connection could linger [#1310](https://github.com/aws/amazon-ecs-agent/pull/1310)

## 1.17.2
* Enhancement - Update the `amazon-ecs-cni-plugins` to `2018.02.0` [#1272](https://github.com/aws/amazon-ecs-agent/pull/1272)
* Enhancement - Add container port mapping and ENI information in introspection
API [#1271](https://github.com/aws/amazon-ecs-agent/pull/1271)

## 1.17.1
* Bug - Fixed a bug that was causing a runtime panic by accessing negative
  index in the health check log slice [#1239](https://github.com/aws/amazon-ecs-agent/pull/1239)
* Bug - Workaround for an issue where CPU percent was set to 1 when CPU was not
  set or set to zero(unbounded) in Windows [#1227](https://github.com/aws/amazon-ecs-agent/pull/1227)
* Bug - Fixed a bug where steady state throttle limits for task metadata endpoints
  were too low for applications [#1240](https://github.com/aws/amazon-ecs-agent/pull/1240)

## 1.17.0
* Feature - Support a HTTP endpoint for `awsvpc` tasks to query metadata
* Feature - Support Docker health check
* Bug - Fixed a bug where `-version` fails due to its dependency on docker
  client [#1118](https://github.com/aws/amazon-ecs-agent/pull/1118)
* Bug - Persist container exit code in agent state file
  [#1125](https://github.com/aws/amazon-ecs-agent/pull/1125)
* Bug - Fixed a bug where the agent could lose track of running containers when
  Docker APIs timeout [#1217](https://github.com/aws/amazon-ecs-agent/pull/1217)
* Bug - Task level memory.use_hierarchy was not being set and memory limits
  were not being enforced [#1195](https://github.com/aws/amazon-ecs-agent/pull/1195)
* Bug - Fixed a bug where CPU utilization wasn't correctly reported on Windows
  [@bboerst](https://github.com/bboerst) [#1219](https://github.com/aws/amazon-ecs-agent/pull/1219)

## 1.16.2
* Bug - Fixed a bug where the ticker would submit empty container state change
  transitions when a task is STOPPED. [#1178](https://github.com/aws/amazon-ecs-agent/pull/1178)

## 1.16.1
* Bug - Fixed a bug where the agent could miss sending an ENI attachment to ECS
  because of address propagation delays. [#1148](https://github.com/aws/amazon-ecs-agent/pull/1148)
* Enhancement - Upgrade the `amazon-ecs-cni-plugins` to `2017.10.1`. [#1155](https://github.com/aws/amazon-ecs-agent/pull/1155)

## 1.16.0
* Feature - Support pulling from Amazon ECR with specified IAM role in task definition
* Feature - Enable support for task level CPU and memory constraints.
* Feature - Enable the ECS agent to run as a Windows service. [#1070](https://github.com/aws/amazon-ecs-agent/pull/1070)
* Enhancement - Support CloudWatch metrics for Windows. [#1077](https://github.com/aws/amazon-ecs-agent/pull/1077)
* Enhancement - Enforce memory limits on Windows. [#1069](https://github.com/aws/amazon-ecs-agent/pull/1069)
* Enhancement - Enforce CPU limits on Windows. [#1089](https://github.com/aws/amazon-ecs-agent/pull/1089)
* Enhancement - Simplify task IAM credential host setup. [#1105](https://github.com/aws/amazon-ecs-agent/pull/1105)

## 1.15.2
* Bug - Fixed a bug where container state information wasn't reported. [#1076](https://github.com/aws/amazon-ecs-agent/pull/1076)

## 1.15.1
* Bug - Fixed a bug where container state information wasn't reported. [#1067](https://github.com/aws/amazon-ecs-agent/pull/1067)
* Bug - Fixed a bug where a task can be blocked in creating state. [#1048](https://github.com/aws/amazon-ecs-agent/pull/1048)
* Bug - Fixed dynamic HostPort in container metadata. [#1052](https://github.com/aws/amazon-ecs-agent/pull/1052)
* Bug - Fixed bug on Windows where container memory limits are not enforced. [#1069](https://github.com/aws/amazon-ecs-agent/pull/1069)

## 1.15.0
* Feature - Support for provisioning tasks with ENIs.
* Feature - Support for `--init` Docker run flag. [#996](https://github.com/aws/amazon-ecs-agent/pull/996)
* Feature - Introduces container level metadata. [#981](https://github.com/aws/amazon-ecs-agent/pull/981)
* Enhancement - Enable 'none' logging driver capability by default.
  [#1041](https://github.com/aws/amazon-ecs-agent/pull/1041)
* Bug - Fixed a bug where tasks that fail to pull containers can cause the agent
  to fail to restore properly after a restart. [#1033](https://github.com/aws/amazon-ecs-agent/pull/1033)
* Bug - Fixed default logging level issue. [#1016](https://github.com/aws/amazon-ecs-agent/pull/1016)
* Bug - Fixed a bug where unsupported Docker API client versions could be registered.
  [#1014](https://github.com/aws/amazon-ecs-agent/pull/1014)
* Bug - Fixed a bug where non-essential container state changes were sometimes not submitted.
  [#1026](https://github.com/aws/amazon-ecs-agent/pull/1026)

## 1.14.5
* Enhancement - Retry failed container image pull operations [#975](https://github.com/aws/amazon-ecs-agent/pull/975)
* Enhancement - Set read and write timeouts for websocket connectons [#993](https://github.com/aws/amazon-ecs-agent/pull/993)
* Enhancement - Add support for the SumoLogic Docker log driver plugin
  [#992](https://github.com/aws/amazon-ecs-agent/pull/992)
* Bug - Fixed a memory leak issue when submitting the task state change [#967](https://github.com/aws/amazon-ecs-agent/pull/967)
* Bug - Fixed a race condition where a container can be created twice when agent restarts. [#939](https://github.com/aws/amazon-ecs-agent/pull/939)
* Bug - Fixed an issue where `microsoft/windowsservercore:latest` was not
  pulled on Windows under certain conditions.
  [#990](https://github.com/aws/amazon-ecs-agent/pull/990)
* Bug - Fixed an issue where task IAM role credentials could be logged to disk. [#998](https://github.com/aws/amazon-ecs-agent/pull/998)

## 1.14.4
* Enhancement - Batch container state change events. [#867](https://github.com/aws/amazon-ecs-agent/pull/867)
* Enhancement - Improve the error message when reserved memory is larger than the available memory. [#897](https://github.com/aws/amazon-ecs-agent/pull/897)
* Enhancement - Allow plain HTTP connections through wsclient. [#899](https://github.com/aws/amazon-ecs-agent/pull/899)
* Enhancement - Support Logentries log driver by [@opsline-radek](https://github.com/opsline-radek). [#870](https://github.com/aws/amazon-ecs-agent/pull/870)
* Enhancement - Allow instance attributes to be provided from config file
  by [@ejholmes](https://github.com/ejholmes). [#908](https://github.com/aws/amazon-ecs-agent/pull/908)
* Enhancement - Reduce the disconnection period to the backend for idle connections. [#912](https://github.com/aws/amazon-ecs-agent/pull/912)
* Bug - Fixed data race where a pointer was returned in Getter. [#889](https://github.com/aws/amazon-ecs-agent/pull/899)
* Bug - Reset agent state if the instance id changed on agent restart. [#892](https://github.com/aws/amazon-ecs-agent/pull/892)
* Bug - Fixed a situation in which containers may be falsely reported as STOPPED
  in the case of a Docker "stop" API failure. [#910](https://github.com/aws/amazon-ecs-agent/pull/910)
* Bug - Fixed typo in log string by [@sharuzzaman](https://github.com/sharuzzaman). [#930](https://github.com/aws/amazon-ecs-agent/pull/930)

## 1.14.3
* Bug - Fixed a deadlock that was caused by the ImageCleanup and Image Pull. [#836](https://github.com/aws/amazon-ecs-agent/pull/836)

## 1.14.2
* Enhancement - Added introspection API for querying tasks by short docker ID, by [@aaronwalker](https://github.com/aaronwalker). [#813](https://github.com/aws/amazon-ecs-agent/pull/813)
* Bug - Added checks for circular task dependencies. [#796](https://github.com/aws/amazon-ecs-agent/pull/796)
* Bug - Fixed an issue with Docker auth configuration overrides. [#751](https://github.com/aws/amazon-ecs-agent/pull/751)
* Bug - Fixed a race condition in the task clean up code path. [#737](https://github.com/aws/amazon-ecs-agent/pull/737)
* Bug - Fixed an issue involving concurrent map writes. [#743](https://github.com/aws/amazon-ecs-agent/pull/743)

## 1.14.1
* Enhancement - Log completion of image pulls. [#715](https://github.com/aws/amazon-ecs-agent/pull/715)
* Enhancement - Increase start and create timeouts to improve reliability under
  some workloads. [#696](https://github.com/aws/amazon-ecs-agent/pull/696)
* Bug - Fixed a bug where throttles on state change reporting could lead to
  corrupted state. [#705](https://github.com/aws/amazon-ecs-agent/pull/705)
* Bug - Correct formatting of log messages from tcshandler. [#693](https://github.com/aws/amazon-ecs-agent/pull/693)
* Bug - Fixed an issue where agent could crash. [#692](https://github.com/aws/amazon-ecs-agent/pull/692)

## 1.14.0
* Feature - Support definition of custom attributes on agent registration.
* Feature - Support Docker on Windows Server 2016.
* Enhancement - Enable concurrent docker pull for docker version >= 1.11.1.
* Bug - Fixes a bug where a task could be prematurely marked as stopped.
* Bug - Fixes an issue where ECS Agent would keep reconnecting to ACS without any backoff.
* Bug - Fix memory metric to exclude cache value.

## 1.13.1
* Enhancement - Added cache for DiscoverPollEndPoint API.
* Enhancement - Expose port 51679 so docker tasks can fetch IAM credentials.
* Bug - fixed a bug that could lead to exhausting the open file limit.
* Bug - Fixed a bug where images were not deleted when using image cleanup.
* Bug - Fixed a bug where task status may be reported as pending while task is running.
* Bug - Fixed a bug where task may have a temporary "RUNNING" state when
  task failed to start.
* Bug - Fixed a bug where CPU metrics would be reported incorrectly for kernel >= 4.7.0.
* Bug - Fixed a bug that may cause agent not report metrics.

## 1.13.0
* Feature - Implemented automated image cleanup.
* Enhancement - Add credential caching for ECR.
* Enhancement - Add support for security-opt=no-new-privileges.
* Bug - Fixed a potential deadlock in dockerstate.

## 1.12.2
* Bug - Fixed a bug where agent keeps fetching stats of stopped containers.

## 1.12.1
* Bug - Fixed a bug where agent keeps fetching stats of stopped containers.
* Bug - Fixed a bug that could lead to exhausting the open file limit.
* Bug - Fixed a bug where the introspection API could return the wrong response code.

## 1.12.0
* Enhancement - Support Task IAM Role for containers launched with 'host' network mode.

## 1.11.1
* Bug - Fixed a bug where telemetry data would fail to serialize properly.
* Bug - Addressed an issue where telemetry would be reported after the
  container instance was deregistered.

## 1.11.0
* Feature - Support IAM roles for tasks.
* Feature - Add support for the Splunk logging driver.
* Enhancement - Reduced pull status verbosity in debug mode.
* Enhancement - Add a Docker label for ECS cluster.
* Bug - Fixed a bug that could cause a container to be marked as STOPPED while
  still running on the instance.
* Bug - Fixed a potential race condition in metrics collection.
* Bug - Resolved a bug where some state could be retained across different
  container instances when launching from a snapshotted AMI.

## 1.10.0
* Feature - Make the `docker stop` timeout configurable.
* Enhancement - Use `docker stats` as the data source for CloudWatch metrics.
* Bug - Fixed an issue where update requests would not be properly acknowledged
  when updates were disabled.

## 1.9.0
* Feature - Add Amazon CloudWatch Logs logging driver.
* Bug - Fixed ACS handler when acking blank message ids.
* Bug - Fixed an issue where CPU utilization could be reported incorrectly.
* Bug - Resolved a bug where containers would not get cleaned up in some cases.

## 1.8.2
* Bug - Fixed an issue where `exec_create` and `exec_start` events were not
  correctly ignored with some Docker versions.
* Bug - Fixed memory utilization computation.
* Bug - Resolved a bug where sending a signal to a container caused the
  agent to treat the container as dead.

## 1.8.1
* Bug - Fixed a potential deadlock in docker_task_engine.

## 1.8.0
* Feature - Task cleanup wait time is now configurable.
* Enhancement - Improved testing for HTTP handler tests.
* Enhancement - Updated AWS SDK to v.1.0.11.
* Bug - Fixed a race condition in a docker-task-engine test.
* Bug - Fixed an issue where dockerID was not persisted in the case of an
  error.

## 1.7.1
* Enhancement - Increase `docker inspect` timeout to improve reliability under
  some workloads.
* Enhancement - Increase connect timeout for websockets to improve reliability
  under some workloads.
* Bug - Fixed memory leak in telemetry ticker loop.

## 1.7.0
* Feature - Add support for pulling from Amazon EC2 Container Registry.
* Bug - Resolved an issue where containers could be incorrectly assumed stopped
  when an OOM event was emitted by Docker.
* Bug - Fixed an issue where a crash could cause recently-created containers to
  become untracked.

## 1.6.0

* Feature - Add experimental HTTP proxy support.
* Enhancement - No longer erroneously store an archive of all logs in the
  container, greatly decreasing memory and CPU usage when rotating at the
  hour.
* Enhancement - Increase `docker create` timeout to improve reliability under
  some workloads.
* Bug - Resolved an issue where private repositories required a schema in
  `AuthData` to work.
* Bug - Fixed issue whereby metric submission could fail and never retry.

## 1.5.0
* Feature - Add support for additional Docker features.
* Feature - Detect and register capabilities.
* Feature - Add -license flag and /license handler.
* Enhancement - Properly handle throttling.
* Enhancement - Make it harder to accidentally expose sensitive data.
* Enhancement - Increased reliability in functional tests.
* Bug - Fixed potential divide-by-zero error with metrics.

## 1.4.0
* Feature - Telemetry reporting for Services and Clusters.
* Bug - Fixed an issue where some network errors would cause a panic.

## 1.3.1
* Feature - Add debug handler for SIGUSR1.
* Enhancement - Trim untrusted cert from CA bundle.
* Enhancement - Add retries to EC2 Metadata fetches.
* Enhancement - Logging improvements.
* Bug - Resolved an issue with ACS heartbeats.
* Bug - Fixed memory leak in ACS payload handler.
* Bug - Fixed multiple deadlocks.

## 1.3.0

* Feature - Add support for re-registering a container instance.

## 1.2.1

* Security issue - Avoid logging configured AuthData at the debug level on startup
* Feature - Add configuration option for reserving memory from the ECS Agent

## 1.2.0
* Feature - UDP support for port bindings.
* Feature - Set labels on launched containers with `task-arn`,
  `container-name`, `task-definition-family`, and `task-definition-revision`.
* Enhancement - Logging improvements.
* Bug - Improved the behavior when CPU shares in a `Container Definition` are
  set to 0.
* Bug - Fixed an issue where `BindIP` could be reported incorrectly.
* Bug - Resolved an issue computing API endpoint when region is provided.
* Bug - Fixed an issue where not specifiying a tag would pull all image tags.
* Bug - Resolved an issue where some logs would not flush on exit.
* Bug - Resolved an issue where some instance identity documents would fail to
  parse.


## 1.1.0
* Feature - Logs rotate hourly and log file names are suffixed with timestamp.
* Enhancement - Improve error messages for containers (visible as 'reason' in
  describe calls).
* Enhancement - Be more permissive in configuration regarding whitespace.
* Enhancement - Docker 1.6 support.
* Bug - Resolve an issue where data-volume containers could result in containers
  stuck in PENDING.
* Bug - Fixed an issue where unknown images resulted in containers stuck in
  PENDING.
* Bug - Correctly sequence task changes to avoid resource contention. For
  example, stopping and starting a container using a host port should work
  reliably now.

## 1.0.0

* Feature - Added the ability to update via ACS when running under
  amazon-ecs-init.
* Feature - Added version information (available via the version flag or the
  introspection API).
* Enhancement - Clarified reporting of task state in introspection API.
* Bug - Fix a lock scoping issue that could cause an invalid checkpoint file
  to be written.
* Bug - Correctly recognize various fatal messages from ACS to error out more
  cleanly.

## 0.0.3 (2015-02-19)

* Feature - Volume support for 'host' and 'empty' volumes.
* Feature - Support for specifying 'VolumesFrom' other containers within a task.
* Feature - Checkpoint state, including ContainerInstance and running tasks, to
  disk so that agent restarts do not leave dangling containers.
* Feature - Add a "/tasks" endpoint to the introspection API.
* Feature - Add basic support for DockerAuth.
* Feature - Remove stopped ECS containers after a few hours.
* Feature - Send a "reason" string for some of the errors that might occur while
  running a container.
* Bug - Resolve several issues where a container would remain stuck in PENDING.
* Bug - Correctly set 'EntryPoint' for containers when configured.
* Bug - Fix an issue where exit codes would not be sent properly.
* Bug - Fix an issue where containers with multiple ports EXPOSEd, but not
  all forwarded, would not start.

## 0.0.2 (2014-12-17)

* Bug - Worked around an issue preventing some tasks to start due to devicemapper
  issues.
