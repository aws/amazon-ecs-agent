# Copyright 2014-2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the
# "License"). You may not use this file except in compliance
# with the License. A copy of the License is located at
#
#     http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is
# distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the
# License for the specific language governing permissions and
# limitations under the License.

%bcond_with systemd # without
%global running_semaphore %{_localstatedir}/run/ecs-init.was-running

%global _cachedir %{_localstatedir}/cache
%global no_exec_perm 644
%global agent_image ecs-agent-v%{version}.tar

Name:           ecs-init
Version:        1.86.2
Release:        1%{?dist}
License:        Apache 2.0
Summary:        Amazon Elastic Container Service initialization application
# TODO: add back aarch64
ExclusiveArch:  x86_64

Source0:        sources.tgz
Source1:        ecs.conf
Source2:        ecs.service
Source3:        amazon-ecs-volume-plugin.service
Source4:        amazon-ecs-volume-plugin.socket
Source5:        amazon-ecs-volume-plugin.conf

BuildRequires:  golang >= 1.7
%if %{with systemd}
BuildRequires:  systemd
Requires:       systemd
%else
Requires:       upstart
%endif
Requires:       iptables
Requires:       docker >= 17.06.2ce
Requires:       procps

%description
ecs-init supports the initialization and supervision of the Amazon ECS
container agent, including configuration of cgroups, iptables, and
required routes among its preparation steps.

%prep
%setup -c

%build
./scripts/get-host-certs
./scripts/build-cni-plugins
./scripts/build true "" false true
./scripts/build-agent-image
./scripts/gobuild.sh

%install
install -D amazon-ecs-init %{buildroot}%{_libexecdir}/amazon-ecs-init
install -D amazon-ecs-volume-plugin %{buildroot}%{_libexecdir}/amazon-ecs-volume-plugin
install -m %{no_exec_perm} -D scripts/amazon-ecs-init.1 %{buildroot}%{_mandir}/man1/amazon-ecs-init.1

mkdir -p %{buildroot}%{_sysconfdir}/ecs
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config.json

# Configure ecs-init to load the built ECS container agent image
mkdir -p %{buildroot}%{_cachedir}/ecs
echo 1 > %{buildroot}%{_cachedir}/ecs/state
install -m %{no_exec_perm} %{agent_image} %{buildroot}%{_cachedir}/ecs/ecs-agent.tar

mkdir -p %{buildroot}%{_sharedstatedir}/ecs/data

%if %{with systemd}
install -m %{no_exec_perm} -D %{SOURCE2} $RPM_BUILD_ROOT/%{_unitdir}/ecs.service
install -m %{no_exec_perm} -D %{SOURCE3} $RPM_BUILD_ROOT/%{_unitdir}/amazon-ecs-volume-plugin.service
install -m %{no_exec_perm} -D %{SOURCE4} $RPM_BUILD_ROOT/%{_unitdir}/amazon-ecs-volume-plugin.socket
%else
install -m %{no_exec_perm} -D %{SOURCE1} %{buildroot}%{_sysconfdir}/init/ecs.conf
install -m %{no_exec_perm} -D %{SOURCE5} %{buildroot}%{_sysconfdir}/init/amazon-ecs-volume-plugin.conf
%endif

%files
%{_libexecdir}/amazon-ecs-init
%{_mandir}/man1/amazon-ecs-init.1*
%{_libexecdir}/amazon-ecs-volume-plugin
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config.json
%{_cachedir}/ecs/ecs-agent.tar
%{_cachedir}/ecs/state
%dir %{_sharedstatedir}/ecs/data

%if %{with systemd}
%{_unitdir}/ecs.service
%{_unitdir}/amazon-ecs-volume-plugin.service
%{_unitdir}/amazon-ecs-volume-plugin.socket
%else
%{_sysconfdir}/init/ecs.conf
%{_sysconfdir}/init/amazon-ecs-volume-plugin.conf
%endif

%post
%if %{with systemd}
%systemd_post ecs
%systemd_post amazon-ecs-volume-plugin.service

%postun
%systemd_postun
%systemd_postun_with_restart amazon-ecs-volume-plugin

%else
%triggerun -- docker
# record whether or not our service was running when docker is upgraded
ecs_status=$(/sbin/status ecs 2>/dev/null || :)
if grep -qF "start/" <<< "${ecs_status}"; then
	/sbin/stop ecs >/dev/null 2>&1 || :
	if [ "$1" -ge 1 ]; then
		# write semaphore if this package is still installed
		touch %{running_semaphore} >/dev/null 2>&1 || :
	fi
fi

%triggerpostun -- docker
# ensures that ecs-init is restarted after docker or ecs-init is upgraded
if [ "$1" -ge 1 ] && [ -e %{running_semaphore} ]; then
	/sbin/start ecs >/dev/null 2>&1 || :
	rm %{running_semaphore} >/dev/null 2>&1 ||:
fi

%postun
# record whether or not our service was running when ecs-init is upgraded
ecs_status=$(/sbin/status ecs 2>/dev/null || :)
if grep -qF "start/" <<< "${ecs_status}"; then
	/sbin/stop ecs >/dev/null 2>&1 || :
	if [ "$1" -ge 1 ]; then
		# write semaphore if this package is upgraded
		touch %{running_semaphore} >/dev/null 2>&1 || :
	fi
fi
# remove semaphore if this package is erased
if [ "$1" -eq 0 ]; then
	rm %{running_semaphore} >/dev/null 2>&1 || :
fi

%triggerun -- ecs-init <= 1.0-3
# handle old ecs-init package that does not properly stop
ecs_status=$(/sbin/status ecs 2>/dev/null || :)
if grep -qF "start/" <<< "${ecs_status}"; then
	/sbin/stop ecs >/dev/null 2>&1 || :
	touch %{running_semaphore} >/dev/null 2>&1 || :
fi

%posttrans
# ensure that we restart after the transaction
if [ -e %{running_semaphore} ]; then
	/sbin/start ecs >/dev/null 2>&1 || :
	rm %{running_semaphore} >/dev/null 2>&1 || :
fi
%endif
