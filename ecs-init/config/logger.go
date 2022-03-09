// Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

// GENERATED FILE, DO NOT MODIFY BY HAND
//go:generate bundle_log_config.sh

package config

// Logger holds the bundled log configuration for seelog
func Logger() string {
	return `
<!--
Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with the
License. A copy of the License is located at

    http://aws.amazon.com/apache2.0/

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
-->
<seelog type="asyncloop">
	<outputs formatid="main">
		<console formatid="console" />
		<rollingfile filename="`+initLogFile()+`" type="date"
			 datepattern="2006-01-02-15" archivetype="zip" maxrolls="5" />
	</outputs>
	<formats>
		<format id="main" format="%UTCDate(2006-01-02T15:04:05Z07:00) [%LEVEL] %Msg%n" />
		<format id="console" format="%UTCDate(2006-01-02T15:04:05Z07:00) %EscM(46)[%LEVEL]%EscM(49) %Msg%n%EscM(0)" />
	</formats>
</seelog>
`
}
