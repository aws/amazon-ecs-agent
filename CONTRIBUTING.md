# Contributing to Amazon ECS Init

Contributions to Amazon ECS Init should be made via GitHub [pull
requests](https://github.com/aws/amazon-ecs-init/pulls) and discussed using
GitHub [issues](https://github.com/aws/amazon-ecs-init/issues).

### Before you start

If you would like to make a significant change, it's a good idea to first open
an issue to discuss it.

### Making the request

Development takes place against the `dev` branch of this repository and pull
requests should be opened against that branch.

Non-code changes, such as updating the README, may be against master if they are
applicable to master.

### Testing

Any contributions should pass all tests.

You may run all test by either running the `make test` target (requires `go`
and `go cover` to be installed) or by running the `make test-in-docker` target
which requires only Docker to be installed.

### Packaging

Amazon ECS Init is officially supported when packaged as an RPM for the Amazon
Linux AMI.  We welcome other packaging contributions in the `packaging` folder.

## Licensing

Amazon ECS Init is released under an [Apache
2.0](http://aws.amazon.com/apache-2-0/) license. Any code you submit will be
released under that license.

For significant changes, we may ask you to sign a [Contributor License
Agreement](http://en.wikipedia.org/wiki/Contributor_License_Agreement).