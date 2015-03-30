# Amazon EC2 Container Service RPM

The Amazon EC2 Container Service RPM is software developed to support the [Amazon ECS Container
Agent](http://github.com/aws/amazon-ecs-agent).  The Amazon ECS RPM is packaged for RPM-based systems that utilize
[Upstart](http://upstart.ubuntu.com) as the init system.

## Behavior
The upstart script installed by the Amazon ECS RPM runs at the completion of runlevel 3, 4, or 5 as the system starts.
The script will clean up any previous copies of the Amazon ECS Container Agent, and then start a new copy.  Logs from
the RPM are available at `/var/log/ecs/ecs-init.log`, while logs from the Amazon ECS Container Agent are available at
`/var/log/ecs/ecs-agent.log`.  The Amazon ECS RPM makes the Amazon ECS Container Agent introspection endpoint available
at `http://127.0.0.1:51678/v1`.  Configuration for the Amazon ECS Container Agent is read from `/etc/ecs/ecs.config`.

## Usage
The upstart script installed by the Amazon EC2 Container Service RPM can be started or stopped with the following commands respectively:

* `sudo start ecs`
* `sudo stop ecs`

### Updates
A naive update method exists to pull tarballs from S3.  Updating at present is very manual, and requires the following
steps:

1. `sudo stop ecs`
2. `sudo /usr/libexec/amazon-ecs-init update-cache`
3. `docker rm ecs-agent`
4. `docker rmi amazon/amazon-ecs-agent:latest`
5. `sudo start ecs`

## License

The Amazon EC2 Container Service RPM is licensed under the Apache 2.0 License.
