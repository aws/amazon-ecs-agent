# Copyright 2014-2022 Amazon.com, Inc. or its affiliates. All Rights Reserved.
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
%global no_exec_perm 644
%global debug_package %{nil}
%global agent_image ecs-agent-v%{version}.tar

Name:           amazon-ecs-init
Version:        1.97.0
Release:        1
License:        Apache 2.0
Summary:        Amazon Elastic Container Service initialization application
ExclusiveArch:  x86_64 aarch64

Source0:        sources.tgz
Source1:        ecs.service
Source2:        amazon-ecs-volume-plugin.service
Source3:        amazon-ecs-volume-plugin.socket

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
make release-agent
./scripts/gobuild.sh %{gobuild_tag}

%install
install -D amazon-ecs-init %{buildroot}%{_libexecdir}/amazon-ecs-init
install -D amazon-ecs-volume-plugin %{buildroot}%{_libexecdir}/amazon-ecs-volume-plugin
install -m %{no_exec_perm} -D scripts/amazon-ecs-init.1 %{buildroot}%{_mandir}/man1/amazon-ecs-init.1

mkdir -p %{buildroot}%{_sysconfdir}/ecs
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config
touch %{buildroot}%{_sysconfdir}/ecs/ecs.config.json

# Configure ecs-init to reload the bundled ECS container agent image.
mkdir -p %{buildroot}%{_cachedir}/ecs
echo 2 > %{buildroot}%{_cachedir}/ecs/state
install -m %{no_exec_perm} %{agent_image} %{buildroot}%{_cachedir}/ecs/

mkdir -p %{buildroot}%{_sharedstatedir}/ecs/data

install -m %{no_exec_perm} -D %{SOURCE1} $RPM_BUILD_ROOT/%{_unitdir}/ecs.service
install -m %{no_exec_perm} -D %{SOURCE2} $RPM_BUILD_ROOT/%{_unitdir}/amazon-ecs-volume-plugin.service
install -m %{no_exec_perm} -D %{SOURCE3} $RPM_BUILD_ROOT/%{_unitdir}/amazon-ecs-volume-plugin.socket

%files
%{_libexecdir}/amazon-ecs-init
%{_mandir}/man1/amazon-ecs-init.1*
%{_libexecdir}/amazon-ecs-volume-plugin
%dir %{_sysconfdir}/ecs
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config
%config(noreplace) %ghost %{_sysconfdir}/ecs/ecs.config.json
%ghost %{_cachedir}/ecs/ecs-agent.tar
%{_cachedir}/ecs/%{basename:%{agent_image}}
%{_cachedir}/ecs/state
%dir %{_sharedstatedir}/ecs/data
%{_unitdir}/ecs.service
%{_unitdir}/amazon-ecs-volume-plugin.service
%{_unitdir}/amazon-ecs-volume-plugin.socket

%post
# Symlink the bundled ECS Agent at loadable path.
ln -sf %{basename:%{agent_image}} %{_cachedir}/ecs/ecs-agent.tar
%systemd_post ecs
%systemd_post amazon-ecs-volume-plugin.service

%postun
%systemd_postun ecs.service
%systemd_postun_with_restart amazon-ecs-volume-plugin

%changelog
* Wed Jul 23 2025 Dane Lim <slimdane@amazon.com> - 1.97.0-1
- Cache Agent version 1.97.0

* Thu Jul 03 2025 Prateek Chaudhry <ptchau@amazon.com> - 1.96.0-1
- Cache Agent version 1.96.0

* Thu Jun 05 2025 Harish Senthilkumar <harishxr@amazon.com> - 1.95.0-1
- Cache Agent version 1.95.0

* Tue May 20 2025 Dane Lim <slimdane@amazon.com> - 1.94.0-1
- Cache Agent version 1.94.0

* Tue May 06 2025 Anuj Singh <singholt@amazon.com> - 1.93.1-1
- Cache Agent version 1.93.1

* Wed Apr 23 2025 Amogh Rathore <amoghr@amazon.com> - 1.93.0-1
- Cache Agent version 1.93.0

* Wed Apr 09 2025 Thean Lim <theanlim@amazon.com> - 1.92.0-1
- Cache Agent version 1.92.0

* Thu Mar 20 2025 Dane H Lim <slimdane@amazon.com> - 1.91.2-1
- Cache Agent version 1.91.2

* Tue Mar 11 2025 Michael Ye <yemike@amazon.com> - 1.91.1-1
- Cache Agent version 1.91.1

* Thu Feb 20 2025 Amogh Rathore <amoghr@amazon.com> - 1.91.0-1
- Cache Agent version 1.91.0

* Thu Jan 23 2025 Thean Lim <theanlim@amazon.com> - 1.90.0-1
- Cache Agent version 1.90.0

* Tue Jan 07 2025 Prateek Chaudhry <ptchau@amazon.com> - 1.89.3-1
- Cache Agent version 1.89.3

* Mon Dec 09 2024 Yiyuan Zhong <yiyzhong@amazon.com> - 1.89.2-1
- Cache Agent version 1.89.2

* Tue Nov 19 2024 Harish Senthilkumar <harishxr@amazon.com> - 1.89.1-1
- Cache Agent version 1.89.1

* Thu Nov 14 2024 Thean Lim <theanlim@amazon.com> - 1.89.0-1
- Cache Agent version 1.89.0

* Wed Oct 30 2024 Michael Ye <yemike@amazon.com> - 1.88.0-1
- Cache Agent version 1.88.0

* Thu Oct 10 2024 Amogh Rathore <amoghr@amazon.com> - 1.87.1-1
- Cache Agent version 1.87.1

* Thu Sep 19 2024 Yiyuan Zhong <yiyzhong@amazon.com> - 1.87.0-1
- Cache Agent version 1.87.0

* Mon Aug 26 2024 Dane Lim <slimdane@amazon.com> - 1.86.3-1
- Cache Agent version 1.86.3

* Sat Aug 17 2024 Harish Senthilkumar <harishxr@amazon.com> - 1.86.2-1
- Cache Agent version 1.86.2

* Thu Aug 15 2024 Harish Senthilkumar <harishxr@amazon.com> - 1.86.1-1
- Cache Agent version 1.86.1

* Wed Aug 07 2024 Dane Lim <slimdane@amazon.com> - 1.86.0-1
- Cache Agent version 1.86.0

* Fri Jul 26 2024 Anuj Singh <singholt@amazon.com> - 1.85.3-1
- Cache Agent version 1.85.3

* Wed Jul 24 2024 Anuj Singh <singholt@amazon.com> - 1.85.2-1
- Cache Agent version 1.85.2

* Thu Jul 11 2024 Harish Senthilkumar <harishxr@amazon.com> - 1.85.1-1
- Cache Agent version 1.85.1

* Mon Jul 08 2024 Amogh Rathore <amoghr@amazon.com> - 1.85.0-1
- Cache Agent version 1.85.0

* Thu Jun 13 2024 Prateek Chaudhry <ptchau@amazon.com> - 1.84.0-1
- Cache Agent version 1.84.0

* Tue May 28 2024 Yiyuan Zhong <yiyzhong@amazon.com> - 1.83.0-1
- Cache Agent version 1.83.0

* Wed May 08 2024 Harish Senthilkumar <harishxr@amazon.com> - 1.82.4-1
- Cache Agent version 1.82.4

* Fri Apr 19 2024 Harish Senthilkumar <harishxr@amazon.com> - 1.82.3-1
- Cache Agent version 1.82.3

* Thu Apr 04 2024 Yiyuan Zhong <yiyzhong@amazon.com> - 1.82.2-1
- Cache Agent version 1.82.2

* Tue Mar 19 2024 Michael Ye <yemike@amazon.com> - 1.82.1-1
- Cache Agent version 1.82.1

* Wed Feb 28 2024 Amogh Rathore <amoghr@amazon.com> - 1.82.0-1
- Cache Agent version 1.82.0

* Tue Feb 20 2024 Yinyi Chen <yinyic@amazon.com> - 1.81.1-1
- Cache Agent version 1.81.1

* Wed Jan 31 2024 Chien-Han Lin <chilinn@amazon.com> - 1.81.0-1
- Cache Agent version 1.81.0

* Tue Jan 09 2024 Michael Ye <yemike@amazon.com> - 1.80.0-1
- Cache Agent version 1.80.0

* Mon Dec 04 2023 Dane Lim <slimdane@amazon.com> - 1.79.2-1
- Cache Agent version 1.79.2

* Mon Nov 13 2023 Chien-Han Lin <chilinn@amazon.com> - 1.79.1-1
- Cache Agent version 1.79.1

* Tue Oct 31 2023 Saisree Valluri <vsaisree@amazon.com> - 1.79.0-1
- Cache Agent version 1.79.0

* Thu Oct 26 2023 Anuj Singh <singholt@amazon.com> - 1.78.1-1
- Cache Agent version 1.78.1

* Wed Oct 11 2023 Cameron Sparr <cssparr@amazon.com> - 1.78.0-1
- Cache Agent version 1.78.0

* Wed Sep 27 2023 Prateek Chaudhry <ptchau@amazon.com> - 1.77.0-1
- Cache Agent version 1.77.0

* Tue Sep 12 2023 Yinyi Chen <yinyic@amazon.com> - 1.76.0-1
- Cache Agent version 1.76.0

* Wed Aug 30 2023 Utsa Bhattacharjya <utsa@amazon.com> - 1.75.3-1
- Cache Agent version 1.75.3

* Tue Aug 29 2023 Utsa Bhattacharjya <utsa@amazon.com> - 1.75.2-1
- Cache Agent version 1.75.2

* Thu Aug 24 2023 Ray Allan <fierlion@amazon.com> - 1.75.1-1
- Cache Agent version 1.75.1

* Wed Aug 09 2023 Chien-Han Lin <chilinn@amazon.com> - 1.75.0-1
- Cache Agent version 1.75.0

* Fri Jul 21 2023 Heming Han <hanhm@amazon.com> - 1.74.1-1
- Cache Agent version 1.74.1

* Thu Jul 20 2023 Heming Han <hanhm@amazon.com> - 1.74.0-1
- Cache Agent version 1.74.0

* Wed Jul 05 2023 Michael Ye <yemike@amazon.com> - 1.73.1-1
- Cache Agent version 1.73.1

* Thu Jun 22 2023 Prateek Chaudhry <ptchau@amazon.com> - 1.73.0-1
- Cache Agent version 1.73.0

* Tue Jun 06 2023 Yinyi Chen <yinyic@amazon.com> - 1.72.0-1
- Cache Agent version 1.72.0

* Tue May 23 2023 Utsa Bhattacharjya <utsa@amazon.com> - 1.71.2-1
- Cache Agent version 1.71.2

* Tue May 09 2023 Heming Han <hanhm@amazon.com> - 1.71.1-1
- Cache Agent version 1.71.1

* Tue Apr 25 2023 Yiyuan Zhong <yiyzhong@amazon.com> - 1.71.0-1
- Cache Agent version 1.71.0

* Tue Apr 11 2023 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.70.2-1
- Cache Agent version 1.70.2

* Tue Mar 28 2023 Yash Kulshrstha <kulshres@amazon.com> - 1.70.1-1
- Cache Agent version 1.70.1

* Mon Mar 13 2023 Utsa Bhattacharjya <utsa@amazon.com> - 1.70.0-1
- Cache Agent version 1.70.0

* Fri Feb 24 2023 Yash Kulshrestha <kulshres@amazon.com> - 1.69.0-1
- Cache Agent version 1.69.0

* Wed Feb 08 2023 Prateek Chaudhry <ptchau@amazon.com> - 1.68.2-1
- Cache Agent version 1.68.2

* Mon Jan 23 2023 Utsa Bhattacharjya <utsa@amazon.com> - 1.68.1-1
- Cache Agent version 1.68.1

* Mon Jan 09 2023 Ray Allan <fierlion@amazon.com> - 1.68.0-1
- Cache Agent version 1.68.0

* Mon Dec 12 2022 Utsa Bhattacharjya <utsa@amazon.com> - 1.67.2-1
- Cache Agent version 1.67.2

* Wed Dec 07 2022 Dane H Lim <slimdane@amazon.com> - 1.67.1-1
- Cache Agent version 1.67.1

* Mon Dec 05 2022 Yash Kulshrestha <kulshres@amazon.com> - 1.67.0-1
- Cache Agent version 1.67.0

* Sat Nov 12 2022 Heming Han <hanhm@amazon.com> - 1.66.2-1
- Cache Agent version 1.66.2

* Thu Nov 10 2022 Heming Han <hanhm@amazon.com> - 1.66.1-1
- Cache Agent version 1.66.1

* Tue Nov 08 2022 Cameron Sparr <cssparr@amazon.com> - 1.66.0-1
- Cache Agent version 1.66.0

* Fri Oct 28 2022 Ray Allan <fierlion@amazon.com> - 1.65.1-1
- Cache Agent version 1.65.1

* Wed Oct 19 2022 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.65.0-1
- Cache Agent version 1.65.0

* Tue Oct 04 2022 Utsa Bhattacharjya <utsa@amazon.com> - 1.64.0-1
- Cache Agent version 1.64.0

* Mon Sep 12 2022 Chien Han Lin <chilinn@amazon.com> - 1.63.1-1
- Cache Agent version 1.63.1
- Dependabot ecs-init fixes

* Tue Sep 06 2022 Chien Han Lin <chilinn@amazon.com> - 1.63.0-1
- Cache Agent version 1.63.0
- Update dependencies to include security patches reported by dependabot for ecs-init
- Fix format string for ecs-init

* Wed Aug 17 2022 Yash Kulshrestha <kulshres@amazon.com> - 1.62.2-1
- Cache Agent version 1.62.2

* Wed Aug 03 2022 Ray Allan <fierlion@amazon.com> - 1.62.1-1
- Fix bug in cgroup mount for rpm builds

* Wed Jul 27 2022 Ray Allan <fierlion@amazon.com> - 1.62.0-1
- Update golang version 1.18.3

* Wed Jun 15 2022 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.61.3-1
- Cache Agent version 1.61.3

* Wed Jun 01 2022 Utsa Bhattacharjya <utsa@amazon.com> - 1.61.2-1
- Cache Agent version 1.61.2

* Tue May 03 2022 Anuj Singh <singholt@amazon.com> - 1.61.1-1
- Cache Agent version 1.61.1
- Install script no longer fails on systems using cgroups v2
- Add GO111MODULE=on to honnef.co/go/tools/cmd/staticcheck

* Tue Apr 05 2022 Cameron Sparr <cssparr@amazon.com> - 1.61.0-1
- Cache Agent version 1.61.0
- Check ipv4 routes for default network interface instead of defaulting to eth0

* Wed Mar 23 2022 Ray Allan <fierlion@amazon.com> - 1.60.1-1
- Cache Agent version 1.60.1

* Wed Mar 02 2022 Chien Han Lin <chilinn@amazon.com> - 1.60.0-1
- Cache Agent version 1.60.0
- Add volume plugin to rpm/deb package

* Fri Feb 04 2022 Yash Kulshrestha <kulshres@amazon.com> - 1.59.0-1
- Cache Agent version 1.59.0
- Log what pre-start is doing

* Fri Jan 14 2022 Utsa Bhattacharjya <utsa@amazon.com> - 1.58.0-2
- Cache Agent version 1.58.0
- Add exec prerequisites to ecs-anywhere installation script

* Fri Dec 03 2021 Mythri Garaga Mannjunatha <mythr@amazon.com> - 1.57.1-1
- Cache Agent version 1.57.1
- Enable AL2 support for ECS-A
- Initialize docker client lazily

* Wed Nov 03 2021 Feng Xiong <fenxiong@amazon.com> - 1.57.0-1
- Cache Agent version 1.57.0

* Thu Oct 21 2021 Cameron Sparr <cssparr@amazon.com> - 1.56.0-1
- Cache Agent version 1.56.0

* Wed Oct 13 2021 Utsa Bhattacharjya <utsa@amazon.com> - 1.55.5-1
- Cache Agent version 1.55.5

* Thu Sep 30 2021 Ray Allan <fierlion@amazon.com> - 1.55.4-1
- Cache Agent version 1.55.4
- GPU updates for ECS Anywhere
- Introduce new configuration variable ECS_OFFHOST_INTROSPECTION_NAME to specify the primary network interface name to block offhost agent introspection port access.

* Thu Sep 16 2021 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.55.3-1
- Cache Agent version 1.55.3

* Thu Sep 02 2021 Yinyi Chen <yinyic@amazon.com> - 1.55.2-1
- Cache Agent version 1.55.2

* Thu Aug 19 2021 Meghna Srivastav <mssrivas@amazon.com> - 1.55.1-1
- Cache Agent version 1.55.1

* Thu Aug 05 2021 Heming Han <hanhm@amazon.com> - 1.55.0-1
- Cache Agent version 1.55.0

* Mon Jul 26 2021 Utsa Bhattacharjya <utsa@amazon.com> - 1.54.1-1
- Cache Agent version 1.54.1

* Wed Jul 07 2021 Ray Allan <fierlion@amazon.com> - 1.54.0-1
- Cache Agent version 1.54.0

* Wed Jun 23 2021 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.53.1-1
- Cache Agent version 1.53.1

* Wed Jun 09 2021 Angel Velazquez <angelcar@amazon.com> - 1.53.0-1
- Cache Agent version 1.53.0

* Tue May 25 2021 Feng Xiong <fenxiong@amazon.com> - 1.52.2-2
- Cache Agent version 1.52.2
- ecs-anywhere-install: fix incorrect download url when running in cn region

* Thu May 20 2021 Feng Xiong <fenxiong@amazon.com> - 1.52.2-1
- Cache Agent version 1.52.2
- ecs-anywhere-install: remove dependency on gpg key server
- ecs-anywhere-install: allow sandboxed apt installations

* Fri May 14 2021 Feng Xiong <fenxiong@amazon.com> - 1.52.1-1
- Cache Agent version 1.52.1

* Wed Apr 28 2021 Ray Allan <fierlion@amazon.com> - 1.52.0-1
- Cache Agent version 1.52.0
- Add support for ECS EXTERNAL launch type (ECS Anywhere)

* Wed Mar 31 2021 Shubham Goyal <shugy@amazon.com> - 1.51.0-1
- Cache Agent version 1.51.0

* Wed Mar 17 2021 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.50.3-1
- Cache Agent version 1.50.3

* Fri Feb 19 2021 Meghna Srivastav <mssrivas@amazon.com> - 1.50.2-1
- Cache Agent version 1.50.2

* Wed Feb 10 2021 Shubham Goyal <shugy@amazon.com> - 1.50.1-1
- Cache Agent version 1.50.1
- Does not restart ECS Agent when it exits with exit code 5

* Fri Jan 22 2021 Utsa Bhattacharjya <utsa@amazon.com> - 1.50.0-1
- Cache Agent version 1.50.0
- Allows ECS customers to execute interactive commands inside containers.

* Wed Jan 06 2021 Shubham Goyal <shugy@amazon.com> - 1.49.0-1
- Cache Agent version 1.49.0
- Removes iptable rule that drops packets to port 51678 unconditionally on ecs service stop

* Mon Nov 23 2020 Shubham Goyal <shugy@amazon.com> - 1.48.1-1
- Cache Agent version 1.48.1

* Thu Nov 19 2020 Shubham Goyal <shugy@amazon.com> - 1.48.0-2
- Cache Agent version 1.48.0

* Fri Oct 30 2020 Mythri Garaga Manjunatha <mythr@amazon.com> - 1.47.0-1
- Cache Agent version 1.47.0

* Fri Oct 16 2020 Meghna Srivastav <mssrivas@amazon.com> - 1.46.0-1
- Cache Agent version 1.46.0

* Wed Sep 30 2020 Cam Sparr <cssparr@amazon.com> - 1.45.0-1
- Cache Agent version 1.45.0
- Block offhost access to agent's introspection port by default. Configurable via env ECS_ALLOW_OFFHOST_INTROSPECTION_ACCESS

* Tue Sep 15 2020 Ray Allan <fierlion@amazon.com> - 1.44.4-1
- Cache Agent version 1.44.4

* Wed Sep 02 2020 Meghna Srivastav  <mssrivas@amazon.com> - 1.44.3-1
- Cache Agent version 1.44.3

* Wed Aug 26 2020 Utsa Bhattacharjya <utsa@amazon.com> - 1.44.2-1
- Cache Agent version 1.44.2

* Thu Aug 20 2020 Shubham Goyal <shugy@amazon.com> - 1.44.1-1
- Cache Agent version 1.44.1

* Thu Aug 13 2020 Shubham Goyal <shugy@amazon.com> - 1.44.0-1
- Cache Agent version 1.44.0
- Add support for configuring Agent container logs

* Tue Aug 04 2020 Feng Xiong <fenxiong@amazon.com> - 1.43.0-2
- Cache Agent version 1.43.0

* Thu Jul 23 2020 Yunhee Lee <yhlee@amazon.com> - 1.42.0-1
- Cache Agent version 1.42.0
- Add a flag ECS_SKIP_LOCALHOST_TRAFFIC_FILTER to allow skipping local traffic filtering

* Thu Jul 09 2020 Feng Xiong <fenxiong@amazon.com> - 1.41.1-2
- Drop traffic to 127.0.0.1 that isn't originated from the host

* Mon Jul 06 2020 Yajie Chu <cya@amazon.com> - 1.41.1-1
- Cache Agent version 1.41.1

* Mon Jun 22 2020 Meghna Srivastav <mssrivas@amazon.com> - 1.41.0-1
- Cache Agent version 1.41.0

* Tue Jun 02 2020 Meghna Srivastav <mssrivas@amazon.com> - 1.40.0-1
- Cache Agent version 1.40.0

* Fri Apr 03 2020 Yunhee Lee <yhlee@amazon.com> - 1.39.0-2
- Cache Agent version 1.39.0
- Ignore IPv6 disable failure if already disabled

* Thu Mar 19 2020 Sharanya Devaraj <sharanyd@amazon.com> - 1.38.0-1
- Cache Agent version 1.38.0
- Adds support for ECS volume plugin
- Disable ipv6 router advertisements for optimization

* Sat Feb 22 2020 Jessie Young <youngli@amazon.com> - 1.37.0-2
- Cache Agent version 1.37.0
- Add '/etc/alternatives' to mounts

* Tue Feb 04 2020 Ray Allan <fierlion@amazon.com> - 1.36.2-1
- Cache Agent version 1.36.2
- update sbin mount point to avoid conflict with Docker >= 19.03.5

* Fri Jan 10 2020 Yunhee Lee <yhlee@amazon.com> - 1.36.1-1
- Cache Agent version 1.36.1

* Wed Jan 08 2020 Cameron Sparr <cssparr@amazon.com> - 1.36.0-1
- Cache Agent version 1.36.0
- capture a fixed tail of container logs when removing a container

* Thu Dec 12 2019 Derek Petersen <petderek@amazon.com> - 1.35.0-1
- Cache Agent version 1.35.0
- Fix bug where stopping agent gracefully would still restart ecs-init

* Mon Nov 11 2019 Shubham Goyal <shugy@amazon.com> - 1.33.0-1
- Cache Agent version 1.33.0
- Fix destination path in docker socket bind mount to match the one specified using DOCKER_HOST on Amazon Linux 2

* Mon Oct 28 2019 Shubham Goyal <shugy@amazon.com> - 1.32.1-1
- Cache Agent version 1.32.1
- Add the ability to set Agent container's labels

* Wed Sep 25 2019 Cameron Sparr <cssparr@amazon.com> - 1.32.0-1
- Cache Agent version 1.32.0

* Fri Sep 13 2019 Yajie Chu <cya@amazon.com> - 1.31.0-1
- Cache Agent version 1.31.0

* Thu Aug 15 2019 Feng Xiong <fenxiong@amazon.com> - 1.30.0-1
- Cache Agent version 1.30.0

* Mon Jul 08 2019 Shubham Goyal <shugy@amazon.com> - 1.29.1-1
- Cache Agent version 1.29.1

* Thu Jun 06 2019 Yumeng Xie <yumex@amazon.com> - 1.29.0-1
- Cache Agent version 1.29.0

* Fri May 31 2019 Feng Xiong <fenxiong@amazon.com> - 1.28.1-2
- Cache Agent version 1.28.1
- Use exponential backoff when restarting agent

* Thu May 09 2019 Feng Xiong <fenxiong@amazon.com> - 1.28.0-1
- Cache Agent version 1.28.0

* Thu Mar 28 2019 Shaobo Han <obo@amazon.com> - 1.27.0-1
- Cache Agent version 1.27.0

* Thu Mar 21 2019 Derek Petersen <petderek@amazon.com> - 1.26.1-1
- Cache Agent version 1.26.1

* Thu Feb 28 2019 Derek Petersen <petderek@amazon.com> - 1.26.0-1
- Cache Agent version 1.26.0
- Add support for running iptables within agent container

* Fri Feb 15 2019 Ray Allan <fierlion@amazon.com> - 1.25.3-1
- Cache Agent version 1.25.3

* Thu Jan 31 2019 Shaobo Han <obo@amazon.com> - 1.25.2-1
- Cache Agent version 1.25.2

* Sat Jan 26 2019 Adnan Khan <adnkha@amazon.com> - 1.25.1-1
- Cache Agent version 1.25.1
- Update ecr models for private link support

* Thu Jan 17 2019 Eric Sun <yuzhusun@amazon.com> - 1.25.0-1
- Cache Agent version 1.25.0
- Add Nvidia GPU support for p2 and p3 instances

* Fri Jan 04 2019 Eric Sun <yuzhusun@amazon.com> - 1.24.0-1
- Cache Agent version 1.24.0

* Fri Nov 16 2018 Jacob Vallejo <jakeev@amazon.com> - 1.22.0-4
- Cache ECS agent version 1.22.0 for x86_64 & ARM
- Support ARM architecture builds

* Thu Nov 15 2018 Jacob Vallejo <jakeev@amazon.com> - 1.22.0-3
- Rebuild

* Fri Nov 02 2018 Yunhee Lee <yhlee@amazon.com> - 1.22.0-2
- Cache Agent version 1.22.0

* Thu Oct 11 2018 Sharanya Devaraj <sharanyd@amazon.com> - 1.21.0-1
- Cache Agent version 1.21.0
- Support configurable logconfig for Agent container to reduce disk usage
- ECS Agent will use the host's cert store on the Amazon Linux platform

* Fri Sep 14 2018 Yumeng Xie <yumex@amazon.com> - 1.20.3-1
- Cache Agent version 1.20.3

* Fri Aug 31 2018 Feng Xiong <fenxiong@amazon.com> - 1.20.2-1
- Cache Agent version 1.20.2

* Thu Aug 09 2018 Peng Yin <penyin@amazon.com> - 1.20.1-1
- Cache Agent version 1.20.1

* Wed Aug 01 2018 Haikuo Liu <haikuo@amazon.com> - 1.20.0-1
- Cache Agent version 1.20.0

* Thu Jul 26 2018 Haikuo Liu <haikuo@amazon.com> - 1.19.1-1
- Cache Agent version 1.19.1

* Thu Jul 19 2018 Feng Xiong <fenxiong@amazon.com> - 1.19.0-1
- Cache Agent version 1.19.0

* Wed May 23 2018 iliana weller <iweller@amazon.com> - 1.18.0-2
- Spec file cleanups
- Enable builds for both AL1 and AL2

* Fri May 04 2018 Haikuo Liu <haikuo@amazon.com> - 1.18.0-1
- Cache Agent version 1.18.0
- Add support for regional buckets
- Bundle ECS Agent tarball in package
- Download agent based on the partition
- Mount Docker plugin files dir

* Fri Mar 30 2018 Justin Haynes <jushay@amazon.com> - 1.17.3-1
- Cache Agent version 1.17.3
- Use s3client instead of httpclient when downloading

* Mon Mar 05 2018 Jacob Vallejo <jakeev@amazon.com> - 1.17.2-1
- Cache Agent version 1.17.2

* Mon Feb 19 2018 Justin Haynes <jushay@amazon.com> - 1.17.1-1
- Cache Agent version 1.17.1

* Mon Feb 05 2018 Justin Haynes <jushay@amazon.com> - 1.17.0-2
- Cache Agent version 1.17.0

* Tue Jan 16 2018 Derek Petersen <petderek@amazon.com> - 1.16.2-1
- Cache Agent version 1.16.2
- Add GovCloud endpoint

* Wed Jan 03 2018 Noah Meyerhans <nmeyerha@amazon.com> - 1.16.1-1
- Cache Agent version 1.16.1
- Improve startup behavior when Docker socket doesn't exist yet

* Tue Nov 21 2017 Noah Meyerhans <nmeyerha@amazon.com> - 1.16.0-1
- Cache Agent version 1.16.0

* Wed Nov 15 2017 Noah Meyerhans <nmeyerha@amazon.com> - 1.15.2-1
- Cache Agent version 1.15.2

* Tue Oct 17 2017 Jacob Vallejo <jakeev@amazon.com> - 1.15.1-1
- Update ECS Agent version

* Sat Oct 07 2017 Justin Haynes <jushay@amazon.com> - 1.15.0-1
- Update ECS Agent version

* Sat Sep 30 2017 Justin Haynes <jushay@amazon.com> - 1.14.5-1
- Update ECS Agent version

* Tue Aug 22 2017 Justin Haynes <jushay@amazon.com> - 1.14.4-1
- Update ECS Agent version

* Thu Jun 01 2017 Adnan Khan <adnkha@amazon.com> - 1.14.2-2
- Cache Agent version 1.14.2
- Add functionality for running agent with userns=host when Docker has userns-remap enabled
- Add support for Docker 17.03.1ce

* Mon Mar 06 2017 Adnan Khan <adnkha@amazon.com> - 1.14.1-1
- Cache Agent version 1.14.1

* Wed Jan 25 2017 Anirudh Aithal <aithal@amazon.com> - 1.14.0-2
- Add retry-backoff for pinging the Docker socket when creating the Docker client

* Mon Jan 16 2017 Derek Petersen <petderek@amazon.com> - 1.14.0-1
- Cache Agent version 1.14.0

* Fri Jan 06 2017 Noah Meyerhans <nmeyerha@amazon.com> - 1.13.1-2
- Update Requires to indicate support for docker <= 1.12.6

* Mon Nov 14 2016 Peng Yin <penyin@amazon.com> - 1.13.1-1
- Cache Agent version 1.13.1

* Tue Sep 27 2016 Noah Meyerhans <nmeyerha@amazon.com> - 1.13.0-1
- Cache Agent version 1.13.0

* Tue Sep 13 2016 Anirudh Aithal <aithal@amazon.com> - 1.12.2-1
- Cache Agent version 1.12.2

* Wed Aug 17 2016 Peng Yin <penyin@amazon.com> - 1.12.1-1
- Cache Agent version 1.12.1

* Wed Aug 10 2016 Anirudh Aithal <aithal@amazon.com> - 1.12.0-1
- Cache Agent version 1.12.0
- Add netfilter rules to support host network reaching credentials proxy

* Wed Aug 03 2016 Samuel Karp <skarp@amazon.com> - 1.11.1-1
- Cache Agent version 1.11.1

* Tue Jul 05 2016 Samuel Karp <skarp@amazon.com> - 1.11.0-1
- Cache Agent version 1.11.0
- Add support for Docker 1.11.2
- Modify iptables and netfilter to support credentials proxy
- Eliminate requirement that /tmp and /var/cache be on the same filesystem
- Start agent with host network mode

* Mon May 23 2016 Peng Yin <penyin@amazon.com> - 1.10.0-1
- Cache Agent version 1.10.0
- Add support for Docker 1.11.1

* Tue Apr 26 2016 Peng Yin <penyin@amazon.com> - 1.9.0-1
- Cache Agent version 1.9.0
- Make awslogs driver available by default

* Thu Mar 24 2016 Juan Rhenals <rhenalsj@amazon.com> - 1.8.2-1
- Cache Agent version 1.8.2

* Mon Feb 29 2016 Juan Rhenals <rhenalsj@amazon.com> - 1.8.1-1
- Cache Agent version 1.8.1

* Wed Feb 10 2016 Juan Rhenals <rhenalsj@amazon.com> - 1.8.0-1
- Cache Agent version 1.8.0

* Fri Jan 08 2016 Samuel Karp <skarp@amazon.com> - 1.7.1-1
- Cache Agent version 1.7.1

* Tue Dec 08 2015 Samuel Karp <skarp@amazon.com> - 1.7.0-1
- Cache Agent version 1.7.0
- Add support for Docker 1.9.1

* Wed Oct 21 2015 Samuel Karp <skarp@amazon.com> - 1.6.0-1
- Cache Agent version 1.6.0
- Updated source dependencies

* Wed Sep 23 2015 Samuel Karp <skarp@amazon.com> - 1.5.0-1
- Cache Agent version 1.5.0
- Improved merge strategy for user-supplied environment variables
- Add default supported logging drivers

* Wed Aug 26 2015 Samuel Karp <skarp@amazon.com> - 1.4.0-2
- Add support for Docker 1.7.1

* Tue Aug 11 2015 Samuel Karp <skarp@amazon.com> - 1.4.0-1
- Cache Agent version 1.4.0

* Thu Jul 30 2015 Samuel Karp <skarp@amazon.com> - 1.3.1-1
- Cache Agent version 1.3.1
- Read Docker endpoint from environment variable DOCKER_HOST if present

* Thu Jul 02 2015 Samuel Karp <skarp@amazon.com> - 1.3.0-1
- Cache Agent version 1.3.0

* Fri Jun 19 2015 Euan Kemp <euank@amazon.com> - 1.2.1-2
- Cache Agent version 1.2.1

* Tue Jun 02 2015 Samuel Karp <skarp@amazon.com> - 1.2.0-1
- Update versioning scheme to match Agent version
- Cache Agent version 1.2.0
- Mount cgroup and execdriver directories for Telemetry feature

* Mon Jun 01 2015 Samuel Karp <skarp@amazon.com> - 1.0-5
- Add support for Docker 1.6.2

* Mon May 11 2015 Samuel Karp <skarp@amazon.com> - 1.0-4
- Properly restart if the ecs-init package is upgraded in isolation

* Wed May 06 2015 Samuel Karp <skarp@amazon.com> - 1.0-3
- Restart on upgrade if already running

* Tue May 05 2015 Samuel Karp <skarp@amazon.com> - 1.0-2
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

