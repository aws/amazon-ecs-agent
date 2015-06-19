# Copyright 2014-2015 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

Name:           ecs-init
Version:        1.2.1
Release:        2%{?dist}
Group:          System Environment/Base
Vendor:         Amazon.com
License:        Apache 2.0
Summary:        Amazon EC2 Container Service initialization application
BuildArch:      x86_64
BuildRoot:      %{_tmppath}/%{name}-%{version}-%{release}-root-%(%{__id_u} -n)

Source0:        sources.tgz
Source1:        ecs.conf

BuildRequires:  golang

Requires:       docker >= 1.6.0, docker <= 1.6.2
Requires:       upstart
Requires(post): docker >= 1.6.0

%global init_dir %{_sysconfdir}/init
%global bin_dir %{_libexecdir}
%global	conf_dir %{_sysconfdir}/ecs
%global cache_dir %{_localstatedir}/cache/ecs
%global data_dir %{_sharedstatedir}/ecs/data
%global man_dir %{_mandir}/man1
%global rpmstate_dir /var/run
%global running_semaphore %{rpmstate_dir}/ecs-init.was-running

%description
ecs-init is a service which may be run to register an EC2 instance as an Amazon
ECS Container Instance.

%prep
%setup -c

%build
./scripts/gobuild.sh
gzip -c scripts/amazon-ecs-init.1 > scripts/amazon-ecs-init.1.gz

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p $RPM_BUILD_ROOT/%{init_dir}
mkdir -p $RPM_BUILD_ROOT/%{bin_dir}
mkdir -p $RPM_BUILD_ROOT/%{conf_dir}
mkdir -p $RPM_BUILD_ROOT/%{cache_dir}
mkdir -p $RPM_BUILD_ROOT/%{data_dir}
mkdir -p $RPM_BUILD_ROOT/%{man_dir}

install %{SOURCE1} $RPM_BUILD_ROOT/%{init_dir}/ecs.conf
install amazon-ecs-init $RPM_BUILD_ROOT/%{bin_dir}/amazon-ecs-init
install scripts/amazon-ecs-init.1.gz $RPM_BUILD_ROOT/%{man_dir}/amazon-ecs-init.1.gz
touch $RPM_BUILD_ROOT/%{conf_dir}/ecs.config
touch $RPM_BUILD_ROOT/%{conf_dir}/ecs.config.json
touch $RPM_BUILD_ROOT/%{cache_dir}/ecs-agent.tar
touch $RPM_BUILD_ROOT/%{cache_dir}/state

%files
%defattr(-,root,root,-)
%{init_dir}/ecs.conf
%{bin_dir}/amazon-ecs-init
%{man_dir}/amazon-ecs-init.1.gz
%config(noreplace) %ghost %{conf_dir}/ecs.config
%config(noreplace) %ghost %{conf_dir}/ecs.config.json
%ghost %{cache_dir}/ecs-agent.tar
%{cache_dir}/state
%dir %{data_dir}

%clean
rm scripts/amazon-ecs-init.1.gz
rm -rf $RPM_BUILD_ROOT

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

%changelog
* Fri Jun 19 2015 Euan Kemp <euank@amazon.com> - 1.2.1-2
- Cache Agent version 1.2.1
* Tue Jun 6 2015 Samuel Karp <skarp@amazon.com> - 1.2.0-1
- Update versioning scheme to match Agent version
- Cache Agent version 1.2.0
- Mount cgroup and execdriver directories for Telemetry feature
* Mon Jun 1 2015 Samuel Karp <skarp@amazon.com> - 1.0-5
- Add support for Docker 1.6.2
* Mon May 11 2015 Samuel Karp <skarp@amazon.com> - 1.0-4
- Properly restart if the ecs-init package is upgraded in isolation
* Wed May 6 2015 Samuel Karp <skarp@amazon.com> - 1.0-3
- Restart on upgrade if already running
* Tue May 5 2015 Samuel Karp <skarp@amazon.com> - 1.0-2
- Cache Agent version 1.1.0
- Add support for Docker 1.6.0
- Force cache load on install/upgrade
- Add man page
* Thu Mar 26 2015 Samuel Karp <skarp@amazon.com> - 1.0-1
- Re-start Agent on non-terminal exit codes
- Enable Agent self-updates
- Cache Agent version 1.0.0
- Added rollback to cached Agent version for failed updates
* Mon Mar 16 2015 Samuel Karp <skarp@amazon.com> - 0.3-0
- Migrate to statically-compiled Go binary
* Tue Feb 17 2015 Eric Nordlund <ericn@amazon.com> - 0.2-3
- Test for existing container agent and force remove it
* Thu Jan 15 2015 Samuel Karp <skarp@amazon.com> - 0.2-2
- Mount data directory for state persistence
- Enable JSON-based configuration
* Mon Dec 15 2014 Samuel Karp <skarp@amazon.com> - 0.2-1
- Naive update functionality
* Thu Dec 11 2014 Samuel Karp <skarp@amazon.com> - 0.2-0
- Initial version
