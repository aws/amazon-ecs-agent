# Contributing to the Amazon ECS Agent

Contributions to the Amazon ECS Agent should be made via GitHub [pull
requests](https://github.com/aws/amazon-ecs-agent/pulls) and discussed using
GitHub [issues](https://github.com/aws/amazon-ecs-agent/issues).

### Before you start

If you would like to make a significant change, it's a good idea to first open
an issue to discuss it.

### Making the request

Development takes place against the `dev` branch of this repository and pull
requests should be opened against that branch.

### Testing

Any contributions should pass all tests, including those not run by our
current CI system.

You may run all test by either running the `make test` target (requires `go`,
and `go cover` to be installed), or by running the `make test-in-docker`
target which requires only Docker to be installed.

## Licensing

The Amazon ECS Agent is released under an [Apache
2.0](http://aws.amazon.com/apache-2-0/) license. Any code you submit will be
released under that license.

For significant changes, we may ask you to sign a [Contributor License
Agreement](http://en.wikipedia.org/wiki/Contributor_License_Agreement).

## Amazon Open Source Code of Conduct

This code of conduct provides guidance on participation in Amazon-managed open source communities, and outlines the process for reporting unacceptable behavior. As an organization and community, we are committed to providing an inclusive environment for everyone. Anyone violating this code of conduct may be removed and banned from the community.

**Our open source communities endeavor to:**
* Use welcoming and inclusive language;
* Be respectful of differing viewpoints at all times;
* Accept constructive criticism and work together toward decisions;
* Focus on what is best for the community and users.

**Our Responsibility.** As contributors, members, or bystanders we each individually have the responsibility to behave professionally and respectfully at all times. Disrespectful and unacceptable behaviors include, but are not limited to:
The use of violent threats, abusive, discriminatory, or derogatory language;
* Offensive comments related to gender, gender identity and expression, sexual orientation, disability, mental illness, race, political or religious affiliation;
* Posting of sexually explicit or violent content;
* The use of sexualized language and unwelcome sexual attention or advances;
* Public or private [harassment](http://todogroup.org/opencodeofconduct/#definitions) of any kind;
* Publishing private information, such as physical or electronic address, without permission;
* Other conduct which could reasonably be considered inappropriate in a professional setting;
* Advocating for or encouraging any of the above behaviors.

**Enforcement and Reporting Code of Conduct Issues.**
Instances of abusive, harassing, or otherwise unacceptable behavior may be reported by contacting opensource-codeofconduct@amazon.com. All complaints will be reviewed and investigated and will result in a response that is deemed necessary and appropriate to the circumstances.

**Attribution.** _This code of conduct is based on the [template](http://todogroup.org/opencodeofconduct) established by the [TODO Group](http://todogroup.org/) and the Scope section from the [Contributor Covenant version 1.4](http://contributor-covenant.org/version/1/4/)._
