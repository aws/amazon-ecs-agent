#!/bin/sh
# Copyright 2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the
# "License"). You may not use this file except in compliance
#  with the License. A copy of the License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and
# limitations under the License.

if [ -z "$1" ] || [ ! -z $(echo "$1" | tr -d "[:digit:]") ]; then
	echo "Must provide numeric argument"
	exit 1
fi

exit_code="$1"

die() {
	exit $exit_code
}

start_wait() {
	echo "Waiting for SIGTERM or SIGINT to exit"
	while true
	do
		sleep 1 &
		wait ${!}
	done
}

SIGINT=2
SIGTERM=15
trap die $SIGINT $SIGTERM
start_wait
