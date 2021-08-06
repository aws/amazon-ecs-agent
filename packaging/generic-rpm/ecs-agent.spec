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


%global gobuild_tag generic_rpm
%global _cachedir %{_localstatedir}/cache
%global bundled_agent_version %{version}
%global no_exec_perm 644

%global ecscni_goproject github.com/aws
%global ecscni_gorepo amazon-ecs-cni-plugins
%global ecscni_goimport %{ecscni_goproject}/%{ecscni_gorepo}
%global ecscni_gitrev 55b2ae77ee0bf22321b14f2d4ebbcc04f77322e1

%global vpccni_goproject github.com/aws
%global vpccni_gorepo amazon-vpc-cni-plugins
%global vpccni_goimport %{vpccni_goproject}/%{vpccni_gorepo}
%global vpccni_gitrev a21d3a41f922e14c19387713df66be3e4ee1e1f6
%global vpccni_gover 1.2

Name:           amazon-ecs-agent
Version:        1.53
Release:        1
License:        Apache 2.0
Summary:        Amazon Elastic Container Service initialization application
ExclusiveArch:  x86_64 aarch64

Source0:        sources.tgz
Source1:        ecs.service
Source2:        https://%{ecscni_goimport}/archive/%{ecscni_gitrev}/%{ecscni_gorepo}.tar.gz
Source3:        https://%{vpccni_goimport}/archive/%{vpccni_gitrev}/%{vpccni_gorepo}.tar.gz
Source4:        amazon-ecs-volume-plugin.service
Source5:        amazon-ecs-volume-plugin.socket

BuildRequires:  systemd
Requires:       systemd
Requires:       iptables
Requires:       procps

%description
containerless agent.

%prep
%setup -c -n amazon-ecs-agent


#unpacking the cni plugins
%setup -T -D -a 2 -q -c -n amazon-ecs-agent/src/github.com/aws
%setup -T -D -a 3 -q -c -n amazon-ecs-agent/src/github.com/aws

mv %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-ecs-cni-plugins* %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-ecs-cni-plugins
mv %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-vpc-cni-plugins* %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-vpc-cni-plugins


%build
%{_builddir}/amazon-ecs-agent/scripts/build false %{gobuild_tag}
%{_builddir}/amazon-ecs-agent/scripts/rpm-volumeBuild.sh 


# Build the ECS CNI plugins
export GOPATH=%{_builddir}/amazon-ecs-agent

cd %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-ecs-cni-plugins
LD_ECS_CNI_VERSION="-X github.com/aws/amazon-ecs-cni-plugins/pkg/version.Version=$(cat VERSION)"
ECS_CNI_HASH="%{ecscni_gitrev}"
LD_ECS_CNI_SHORT_HASH="-X github.com/aws/amazon-ecs-cni-plugins/pkg/version.GitShortHash=${ECS_CNI_HASH::8}"
LD_ECS_CNI_PORCELAIN="-X github.com/aws/amazon-ecs-cni-plugins/pkg/version.GitPorcelain=0"
go build -a \
  -buildmode=pie \
  -ldflags "-linkmode=external ${LD_ECS_CNI_VERSION} ${LD_ECS_CNI_SHORT_HASH} ${LD_ECS_CNI_PORCELAIN}" \
  -o ecs-eni \
  ./plugins/eni
go build -a \
  -buildmode=pie \
  -ldflags "-linkmode=external ${LD_ECS_CNI_VERSION} ${LD_ECS_CNI_SHORT_HASH} ${LD_ECS_CNI_PORCELAIN}" \
  -o ecs-ipam \
  ./plugins/ipam
go build -a \
  -buildmode=pie \
  -ldflags "-linkmode=external ${LD_ECS_CNI_VERSION} ${LD_ECS_CNI_SHORT_HASH} ${LD_ECS_CNI_PORCELAIN}" \
  -o ecs-bridge \
  ./plugins/ecs-bridge

cd %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-vpc-cni-plugins
LD_VPC_CNI_VERSION="-X github.com/aws/amazon-vpc-cni-plugins/version.Version=%{vpccni_gover}"
VPC_CNI_HASH="%{vpccni_gitrev}"
LD_VPC_CNI_SHORT_HASH="-X github.com/aws/amazon-vpc-cni-plugins/version.GitShortHash=${VPC_CNI_HASH::8}"
go build -a \
  -buildmode=pie \
  -ldflags "-linkmode=external ${LD_VPC_CNI_VERSION} ${LD_VPC_CNI_SHORT_HASH} ${LD_VPC_CNI_PORCELAIN}" \
  -mod=vendor \
  -o vpc-branch-eni \
  ./plugins/vpc-branch-eni

cd ..


%install
install -D %{_builddir}/amazon-ecs-agent/generic_rpm %{buildroot}%{_libexecdir}/amazon-ecs-agent
install -D %{_builddir}/amazon-ecs-agent/amazon-ecs-volume-plugin %{buildroot}%{_libexecdir}/amazon-ecs-volume-plugin
install -D %{_topdir}/packaging/generic-rpm/ipSetup.sh %{buildroot}%{_libexecdir}/ipSetup.sh
install -D %{_topdir}/packaging/generic-rpm/ipCleanup.sh %{buildroot}%{_libexecdir}/ipCleanup.sh
install -D %{_topdir}/out/amazon-ecs-pause.tar %{buildroot}%{_sharedstatedir}/amazon-ecs-pause.tar
install -D -p -m 0755 %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-ecs-cni-plugins/ecs-bridge %{buildroot}%{_libexecdir}/ecs-bridge
install -D -p -m 0755 %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-ecs-cni-plugins/ecs-eni %{buildroot}%{_libexecdir}/ecs-eni
install -D -p -m 0755 %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-ecs-cni-plugins/ecs-ipam %{buildroot}%{_libexecdir}/ecs-ipam
install -D -p -m 0755 %{_builddir}/amazon-ecs-agent/src/github.com/aws/amazon-vpc-cni-plugins/vpc-branch-eni %{buildroot}%{_libexecdir}/vpc-branch-eni

mkdir -p %{buildroot}%{_sysconfdir}/ecs
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config.json

mkdir -p %{buildroot}%{_sharedstatedir}/ecs/data

install -m %{no_exec_perm} -D %{SOURCE1} $RPM_BUILD_ROOT/%{_unitdir}/ecs.service
install -m %{no_exec_perm} -D %{SOURCE4} $RPM_BUILD_ROOT/%{_unitdir}/amazon-ecs-volume-plugin.service
install -m %{no_exec_perm} -D %{SOURCE5} $RPM_BUILD_ROOT/%{_unitdir}/amazon-ecs-volume-plugin.socket

%files
%{_libexecdir}/amazon-ecs-agent
%{_libexecdir}/amazon-ecs-volume-plugin
%{_libexecdir}/ipSetup.sh
%{_libexecdir}/ipCleanup.sh
%{_libexecdir}/ecs-bridge
%{_libexecdir}/ecs-eni
%{_libexecdir}/ecs-ipam
%{_libexecdir}/vpc-branch-eni
%{_sharedstatedir}/amazon-ecs-pause.tar
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config.json
%dir %{_sharedstatedir}/ecs/data
%{_unitdir}/ecs.service
%{_unitdir}/amazon-ecs-volume-plugin.service
%{_unitdir}/amazon-ecs-volume-plugin.socket

%post
%systemd_post ecs

%postun
%systemd_postun

