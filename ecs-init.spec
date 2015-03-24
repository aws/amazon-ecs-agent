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

Name:          ecs-init
Version:       0.3
Release:       0%{?dist}
Group:         System Environment/Base
Vendor:        Amazon.com
License:       Apache 2.0
Summary:       Amazon EC2 Container Service initialization application
BuildArch:     x86_64
BuildRoot:     %{_tmppath}/%{name}-%{version}-%{release}-root-%(%{__id_u} -n)

Source0:       sources.tgz
Source1:       ecs.conf

BuildRequires: golang

Requires:      docker >= 1.3.0, docker <= 1.5.0
Requires:      upstart

%global init_dir %{_sysconfdir}/init
%global bin_dir %{_libexecdir}
%global	conf_dir %{_sysconfdir}/ecs
%global cache_dir %{_localstatedir}/cache/ecs
%global data_dir %{_sharedstatedir}/ecs/data

%description
ecs-init is a service which may be run to register an EC2 instance as an Amazon
ECS Container Instance.

%prep
%setup -c

%build
./scripts/gobuild.sh

%install
rm -rf $RPM_BUILD_ROOT
mkdir -p $RPM_BUILD_ROOT/%{init_dir}
mkdir -p $RPM_BUILD_ROOT/%{bin_dir}
mkdir -p $RPM_BUILD_ROOT/%{conf_dir}
mkdir -p $RPM_BUILD_ROOT/%{cache_dir}
mkdir -p $RPM_BUILD_ROOT/%{data_dir}

install %{SOURCE1} $RPM_BUILD_ROOT/%{init_dir}/ecs.conf
install amazon-ecs-init $RPM_BUILD_ROOT/%{bin_dir}/amazon-ecs-init
touch $RPM_BUILD_ROOT/%{conf_dir}/ecs.config
touch $RPM_BUILD_ROOT/%{conf_dir}/ecs.config.json
touch $RPM_BUILD_ROOT/%{cache_dir}/ecs-agent.tar

%files
%defattr(-,root,root,-)
%{init_dir}/ecs.conf
%{bin_dir}/amazon-ecs-init
%config(noreplace) %ghost %{conf_dir}/ecs.config
%config(noreplace) %ghost %{conf_dir}/ecs.config.json
%ghost %{cache_dir}/ecs-agent.tar
%dir %{data_dir}

%clean
rm -rf $RPM_BUILD_ROOT

%changelog
* Mon Mar 16 2015 Samuel Karp <skarp@amazon.com) - 0.3-0
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
