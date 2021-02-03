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

%global gobuild_tag generic-rpm
%global _cachedir %{_localstatedir}/cache
%global bundled_agent_version %{version}
%global no_exec_perm 644

%ifarch x86_64
%global agent_image %{SOURCE3}
%endif
%ifarch aarch64
%global agent_image %{SOURCE4}
%endif

Name:           amazon-ecs-init
Version:        1.48.1
Release:        1
License:        Apache 2.0
Summary:        Amazon Elastic Container Service initialization application
ExclusiveArch:  x86_64 aarch64

Source0:        sources.tgz
Source1:        ecs.conf
Source2:        ecs.service
# x86_64 Container agent docker image
Source3:        https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-v%{bundled_agent_version}.tar
# aarch64 Container agent docker image
Source4:        https://s3.amazonaws.com/amazon-ecs-agent/ecs-agent-arm64-v%{bundled_agent_version}.tar

BuildRequires:  golang >= 1.7
BuildRequires:  systemd
Requires:       systemd
Requires:       iptables
Requires:       procps

%description
ecs-init supports the initialization and supervision of the Amazon ECS
container agent, including configuration of cgroups, iptables, and
required routes among its preparation steps.

%prep
%setup -c

%build
./scripts/gobuild.sh %{gobuild_tag}

%install
install -D amazon-ecs-init %{buildroot}%{_libexecdir}/amazon-ecs-init
install -m %{no_exec_perm} -D scripts/amazon-ecs-init.1 %{buildroot}%{_mandir}/man1/amazon-ecs-init.1

mkdir -p %{buildroot}%{_sysconfdir}/ecs
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config.json

# Configure ecs-init to reload the bundled ECS container agent image.
mkdir -p %{buildroot}%{_cachedir}/ecs
echo 2 > %{buildroot}%{_cachedir}/ecs/state
# Add a bundled ECS container agent image
install -m %{no_exec_perm} %{agent_image} %{buildroot}%{_cachedir}/ecs/

mkdir -p %{buildroot}%{_sharedstatedir}/ecs/data

install -m %{no_exec_perm} -D %{SOURCE2} $RPM_BUILD_ROOT/%{_unitdir}/ecs.service

%files
%{_libexecdir}/amazon-ecs-init
%{_mandir}/man1/amazon-ecs-init.1*
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config.json
%ghost %{_cachedir}/ecs/ecs-agent.tar
%{_cachedir}/ecs/%{basename:%{agent_image}}
%{_cachedir}/ecs/state
%dir %{_sharedstatedir}/ecs/data
%{_unitdir}/ecs.service

%post
# Symlink the bundled ECS Agent at loadable path.
ln -sf %{basename:%{agent_image}} %{_cachedir}/ecs/ecs-agent.tar
%systemd_post ecs

%postun
%systemd_postun

%changelog