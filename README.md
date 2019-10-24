# Amazon Elastic Container Service RPM

[![Build Status](https://travis-ci.org/aws/amazon-ecs-init.svg?branch=master)](https://travis-ci.org/aws/amazon-ecs-init)

The Amazon Elastic Container Service RPM is software developed to support the [Amazon ECS Container
Agent](http://github.com/aws/amazon-ecs-agent).  The Amazon ECS RPM is packaged for RPM-based systems that utilize
[Upstart](http://upstart.ubuntu.com) as the init system.

## Behavior
The upstart script installed by the Amazon ECS RPM runs at the completion of runlevel 3, 4, or 5 as the system starts.
The script will clean up any previous copies of the Amazon ECS Container Agent, and then start a new copy.  Logs from
the RPM are available at `/var/log/ecs/ecs-init.log`, while logs from the Amazon ECS Container Agent are available at
`/var/log/ecs/ecs-agent.log`.  The Amazon ECS RPM makes the Amazon ECS Container Agent introspection endpoint available
at `http://127.0.0.1:51678/v1`.  Configuration for the Amazon ECS Container Agent is read from `/etc/ecs/ecs.config`.
All of the configurations in this file are used as environment variables of the ECS Agent container. Additionally, some 
configurations can be used to configure other properties of the ECS Agent container, as described below.

| Configuration Key | Example Value(s)            | Description | Default value |
|:----------------|:----------------------------|:------------|:-----------------------|
| `ECS_AGENT_LABELS` | `{"test.label.1":"value1","test.label.2":"value2"}` | The labels to add to the ECS Agent container. | |

## Usage
The upstart script installed by the Amazon Elastic Container Service RPM can be started or stopped with the following commands respectively:

* `sudo start ecs`
* `sudo stop ecs`

### Updates
Updates to the Amazon ECS Container Agent should be performed through the Amazon ECS Container Agent.  In the case where
an update failed and the Amazon ECS Container Agent is no longer functional, a rollback can be initiated as follows:

1. `sudo stop ecs`
2. `sudo /usr/libexec/amazon-ecs-init reload-cache`
3. `sudo start ecs`

## Security disclosures
If you think you’ve found a potential security issue, please do not post it in the Issues.  Instead, please follow the instructions [here](https://aws.amazon.com/security/vulnerability-reporting/) or [email AWS security directly](mailto:aws-security@amazon.com).

## License

The Amazon Elastic Container Service RPM is licensed under the Apache 2.0 License.
