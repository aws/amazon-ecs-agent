---
inclusion: always
---

# Implementation Guide

## Project Structure

This is a multi-module Go repository for the Amazon ECS Agent. The three primary modules are:

- `ecs-agent/` — shared library code (credentials, networking, logging, API models, utilities). This is the lowest-level module and must not import from `agent/`.
- `agent/` — the main ECS agent binary. Imports `ecs-agent/` via a `replace` directive (treated as a local dependency, not a vendored one).
- `ecs-init/` — the init/systemd process that manages the agent container lifecycle.

Each module vendors its own dependencies (`go mod vendor`). A local fork of `aws-sdk-go-v2/service/ecs` lives in `aws-sdk-go-v2/` and is pulled in via `replace` directives.

## Code Style

- Import groups separated by blank lines: (1) standard library, (2) local (`github.com/aws/amazon-ecs-agent/...`), (3) third-party vendor. The `agent/` module imports `ecs-agent/` packages in the local group, not the vendor group.
- Run `goimports` and `gofmt` before committing. CI verifies no diff is generated.
- Line length should not exceed 100 characters.
- Write comments as complete sentences with punctuation.
- No sensitive data in logs (credentials, keys, tokens).
- Use `aws-sdk-go-v2`, not v1, for new code.

## Logging

- Use `ecs-agent/logger` (structured logging) for new code, not `seelog`.
- Use predefined field constants from `ecs-agent/logger/field` (e.g., `field.TaskID`, `field.Error`) instead of raw strings.
- Logging calls use `logger.Fields{}` maps: `logger.Info("message", logger.Fields{field.TaskID: id})`.

## Dependency Management

- Run `make gomod` to tidy and re-vendor all modules.

## License Header

Every `.go` file must start with the Apache 2.0 license header:

```go
// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.
```
