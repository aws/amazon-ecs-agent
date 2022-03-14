# Update Changelog

## Be sure your git is clean
We'll be relying on git to guard us against problems.
Staring from a clean checkout follow these steps.

## Update CHANGELOG\_MASTER
The CHANGELOG\_MASTER consists of

version (Name.Version.Release)
User <user@email>
Datetime(RFC1123 format)
List of changes separated by newline
Newline

```
1.32.1-1
Shubham Goyal <shugy@amazon.com>
Mon, 28 Oct 2019 09:00:00 PST
Cache Agent version 1.32.1
Add the ability to set Agent container's labels
```

You'll need to add a new entry to the CHANGELOG\_MASTER before running the
chagelog.go script

## Build and run changelog.go
From the scripts/changelog dir, build and run the changelog binary with `go run ./changelog.go`

use git status to check that the following files are updated
```
modified:   ../../packaging/amazon-linux-ami/ecs-init.spec
modified:   ../../packaging/ubuntu-trusty/debian/changelog
modified:   ../../packaging/suse/amazon-ecs-init.changes
modified:   ../../CHANGELOG.md
```
use `git diff` to be sure that the change is what you expect.  Pay special
attention to the date.

add and commit the updated changelog files and then delete the built changelog
binary.
