mackerel-plugin-aws-lambda [![Build Status](https://travis-ci.org/astj/mackerel-plugin-aws-lambda.svg?branch=master)](https://travis-ci.org/astj/mackerel-plugin-aws-lambda)
=================================

AWS Lambda custom metrics plugin for mackerel.io agent.

## DEPRECATED

This plugin is now part of [Official Mackerel Agent Plugins](https://github.com/mackerelio/mackerel-agent-plugins/tree/master/mackerel-plugin-aws-lambda).
Please use official plugins, not this repository.

## Synopsis

```shell
mackerel-plugin-aws-lambda [-function-name=<function-name>] -region=<aws-region> -access-key-id=<id> -secret-access-key=<key> [-tempfile=<tempfile>] [-metric-key-prefix=<prefix>]
```
* If `function-name` is supplied, collect data from specified Lambda function.
  * If not, whole Lambda stastics in the region is collected.
* you can set some parameters by environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`.
  * If both of those environment variables and command line parameters are passed, command line parameters are used.
* You may omit `region` parameter if you're running this plugin on an EC2 instance running in same region with the target Lambda function

## Example of mackerel-agent.conf

```
[plugin.metrics.aws-lambda]
command = "/path/to/mackerel-plugin-aws-lambda -function-name=MyFunc -region=ap-northeast-1"
```

## License

Copyright 2016 astj

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the License. You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions and limitations under the License.
