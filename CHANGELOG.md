# Changelog

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
