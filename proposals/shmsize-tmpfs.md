<!--
Copyright 2017 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may
not use this file except in compliance with the License. A copy of the
License is located at

     http://aws.amazon.com/apache2.0/

or in the "license" file accompanying this file. This file is distributed
on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
express or implied. See the License for the specific language governing
permissions and limitations under the License.
-->

# Support for ShmSize and Tmpfs Docker Parameters in ECS

## Background

Docker supports the parameters `--shm-size` and `--tmpfs` which sets the size of
`/dev/shm` volume and creates an empty tmpfs mount within a container respectively.
More information on these flags:
* ShmSize: https://docs.docker.com/engine/reference/run/#runtime-constraints-on-resources
* Tmpfs: https://docs.docker.com/engine/reference/run/#tmpfs-mount-tmpfs-filesystems

## ECS Support for the flags

### ShmSize
A new field will be added for Docker's --shm-size flag under the linuxParameters
field in ContainerDefinition of the Task definition. Through this field, customers
will be able to set the value for `ShmSize` parameter in Docker's HostConfig that
is passed during create container call.

### Tmpfs
A new field will be added for Docker's --tmpfs flag under the linuxParameters field
in ContainerDefinition of the Task definition.  Through this field, customers will
be able to set the value for `Tmpfs` parameter in Docker's HostConfig that is passed
during the create container call.

Tmpfs parameter takes in a container path as the mount point and other mount options
for tmpfs including the mount size. Other options can be found in the man page for
mount [here](http://man7.org/linux/man-pages/man8/mount.8.html).

## Implications of using these fields during task placement decisions:

Since both the flags consume memory in the instance in which their corresponding
tasks are placed, their memory sizes will be accounted against the total memory
available in the instance. As a result, the memory set for both the ShmSize and
Tmpfs parameters will affect placement decisions for other tasks in the same instances.
Size will be a mandatory parameter in the Tmpfs field since it is essential for
making task placement decisions.

Memory reservation metric would now include the `shmsize` and size of `tmpfs` mounts added
to the containers through the task definition. The memory utilization metric remains
the same as it is today.
Example:
If 80 MiB of container memory is allocated through `memory` parameter and 20 MiB
for `ShmSize`, then the reservation metric would reflect the total memory reserved
as 100 MiB. Suppose the container uses the entire 80 MiB of container memory and
none of the shared memory, the utilization metrics would still show the usage as
100%, since it is not affected by the memory reserved/used for shared memory (`ShmSize`).

## Validation of the new fields:

* ShmSize: This should be a non negative integer value in MiB.
* Tmpfs:
    * Container Path: This should be an absolute file path and will be a required
    field. This path should not conflict with any of the container mount paths for
    volumes.
    * Size: Non negative integer value in MiB. This will be a required field.
    * Mount options: This should be one of the mount options for `tmpfs` allowed
    by Docker (except size).
