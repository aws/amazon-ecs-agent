# Building the ECS Agent using AWS CodeSuite

## Prerequisites

There are some things you need to get this going

1. Your favorite AWS region
1. A GPG key stored in AWS Secrets Manager
1. A passphrase for the aforementioned GPG key, also stored on AWS Secrets Manager
1. A codestar github connection
1. A final artifacts bucket

## How to get the key and the connection

Generate and export a key for yourself by running the following commands.

```shell
gpg --full-generate-key
gpg --output private.gpg --armor --export-secret-key <email>
```

A couple of things to keep in mind. If you're using Amazon Linux, it comes with a slightly older `gpg` executable so it doesn't _yet_ support ECC keys; best bet is to pick RSA 4096-bit. You will also need a passphrase for the key because it's better security.

You will then have to import this key into secrets manager. To set expectations, plaintext in the context of creating secrets means unstructured (non JSON formatted) data, rather than unencrypted data. Be sure to pick plaintext and just paste the contents of the key into the textbox without JSON encoding it. Secrets Manager encourages the use of JSON structured data but it is not required. You will also need to create a Secrets Manager secret for the passphrase the same way as above.

As for the codestar connection, you can generate one of those by logging into the AWS Console, going to any of the CodeSuite services, clicking on the Settings left nav item and clicking connections. And then clicking the new button. You'll need the ARN of that connection.

## Release pipeline

Grab the `release-pipeline-stack.yml` file and either upload it to CloudFormation or paste it in the CloudFormation designer and create a stack. The stack will ask you for some of the information that you've created above and will generate the following resources

1. A CloudWatch Logs group for the whole CodePipeline
1. An S3 bucket used to move artifacts between the CodePipeline stages
1. A builder CodeBuild project and corresponding IAM role
1. A signer CodeBuild project and corresponding IAM role
1. A copier CodeBuild project and corresponding IAM role
1. A CodePipeline to pull these all together and corresponding IAM role

Once the stack is successful, by default, the project is configured to use `buildspecs/<stage>.yml` as the CodeBuild buildspec file but you can change that by manually editing the buildspec for each CodeBuild project or putting the buildspec files in this repository in a `buildspecs` folder in the root of the repository you're building.

The directory structure that is expected is as follows,

```
buildspecs/
|- merge-build.yml
|- signing.yml
|- copy.yml
```

Everything else is already set up for you.

## Adding a new artifact get signed and copied

There are a few changes that need to be made to add another artifact that needs to be signed and copied to the CodePipeline. They are as follows,

1. The CodeBuild project that feeds into the signer has to export a variable that carries the name of the artifact to sign. Check out the `exported-variables` key within `merge-build.yml` for an example of how to set this up. You create an environment variable and any defined environment variable can be exported from a codebuild project.
1. In `release-pipeline-stack.yml`, add the project that will feed into the signer, and add an IAM role to go with it. See the `AmdBuildCodeBuildProjectServiceRole` and `AmdBuildCodeBuildProject` keys in the file for examples. Follow established naming conventions.
1. In `release-pipeline-stack.yml`, add an action to the appropriate stage under the `BuildAndSignCodePipeline` key. Likely, you're going to want to define a new item in the `Actions` list under one of the `Stages`. Pay close attention to define both `OutputArtifacts` and `Namespace` for your new action. See `Action` with the `Name` called `MakeAmd` for an example.
1. In `release-pipeline-stack.yml`, add a new entry in the `InputArtifacts` list for the `Action` with `Name` called `GPG`. This name should correspond to the `OutputArtifacts` name that you defined above.
1. In `release-pipeline-stack.yml`, the `Action` with `Name` called `GPG` has a key called `EnvironmentVariables` with a JSON string as value. Be careful not to unstringify the JSON, CodePipeline through CloudFormation expects this key to have stringified JSON, and nothing else. Add an entry to this declaring the new artifact that you want to sign. Your exported variable from the CodeBuild buildspec and the `Namespace` that you declared comes into play here. You can find the name of the artifact in the value specified by `#{<namespace_name>.<exported_variable_name>}`. Create a new JSON object with the `name` key containing the environment variable name you'd like to be set, the `value` key containing something akin to `#{something.something}`, and the `type` key set to `PLAINTEXT`.
1. Within the signing CodeBuild environment, the primary source is the directory that the build starts out in. All the other input sources are what CodeBuild considers secondary sources and they get their own directories within the build environment. Those directories are stored in environment variables in the format `$CODEBUILD_SRC_DIR_<output_artifact_name>`. This allows you to locate the new artifact that needs to be signed and call the `sign_file` function with the new artifact.
1. The signing is going to generate a new file that ends in `.asc`. You have to export that out of the signing environment for it to be picked up by the Copy to S3 CodeBuild project.
1. The Copy to S3 CodeBuild project is already written to handle multiple files so no changes are required there.

## Secrets Manager access logs

There is a separate template called `audit-logs-stack.yml` that contains audit logging for the key stored in AWS Secrets Manager. You can use CloudTrail to find the `GetSecretValue` events using the Event Name filter or using `secretsmanager.amazonaws.com` as the Event Source. This applies for the last 90 days.

The events also get delivered to CloudWatch Logs to act as an archive for the last 180 days by default but can be configured to keep those events around for longer if necessary.
