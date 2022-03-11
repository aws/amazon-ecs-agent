#
# spec file for package amazon-ecs-init
#
# Copyright (c) 2015 SUSE LINUX Products GmbH, Nuernberg, Germany.
#
# All modifications and additions to the file contributed by third parties
# remain the property of their copyright owners, unless otherwise agreed
# upon. The license for this file, and modifications and additions to the
# file, is the same license as for the pristine package itself (unless the
# license for the pristine package is not an Open Source License, in which
# case the license is the MIT License). An "Open Source License" is a
# license that conforms to the Open Source Definition (Version 1.9)
# published by the Open Source Initiative.

# Please submit bugfixes or comments via http://bugs.opensuse.org/
#
# TODO: replace with generic rpm. Need to verify the latter works on EC2.

%define short_name amazon-ecs
Name:           amazon-ecs-init
Version:        1.7.1
Release:        0
Summary:        Amazon Elastic Container Service Initialization
License:        Apache-2.0
Group:          System Environment/Base
Url:            https://github.com/aws/amazon-ecs-agent
Source0:        %{name}-%{version}.tar.gz
Source1:        %{short_name}.service
BuildRequires:  go
BuildRequires:  systemd
Requires:       docker >= 1.6.0
Requires:       systemd
BuildRoot:      %{_tmppath}/%{name}-%{version}-build
ExclusiveArch:  %ix86 x86_64

%description
The Amazon Elastic Container Service initialization will start the ECS agent.
The ECS agent runs in a container and is needed to support integration
between the aws-cli ecs command line tool and an instance running in
Amazon EC2.

%prep
%setup -q -n %{name}-%{version}

%build
./scripts/gobuild.sh suse
gzip -c scripts/amazon-ecs-init.1 > scripts/amazon-ecs-init.1.gz

%install
install -d -m 755 %{buildroot}/%{_mandir}/man1
install -d -m 755 %{buildroot}/%{_sbindir}
install -d -m 755 %{buildroot}/%{_sysconfdir}/ecs
install -m 644 scripts/amazon-ecs-init.1.gz %{buildroot}/%{_mandir}/man1
install -m 755 amazon-ecs-init %{buildroot}/%{_sbindir}

mkdir -p %{buildroot}/%{_unitdir}
install -m 755 %SOURCE1 %{buildroot}/%{_unitdir}

touch %{buildroot}/%{_sysconfdir}/ecs/ecs.config
touch %{buildroot}/%{_sysconfdir}/ecs/ecs.config.json

mkdir -p %{buildroot}/%{_localstatedir}/cache/ecs
touch %{buildroot}/%{_localstatedir}/cache/ecs/ecs-agent.tar
touch %{buildroot}/%{_localstatedir}/cache/ecs/state

%files
%defattr(-,root,root,-)
%dir %{_sysconfdir}/ecs
%dir %{_localstatedir}/cache/ecs
%doc CONTRIBUTING.md LICENSE NOTICE README.md
%config(noreplace) %{_sysconfdir}/ecs/ecs.config
%config(noreplace) %{_sysconfdir}/ecs/ecs.config.json
%{_mandir}/man*/*
%{_sbindir}/*
%{_unitdir}/%{short_name}.service
%{_localstatedir}/cache/ecs/ecs-agent.tar
%{_localstatedir}/cache/ecs/state


%pre
%service_add_pre %{short_name}.service

%preun
%service_del_preun %{short_name}.service

%post
%service_add_post %{short_name}.service

%postun
%service_del_postun %{short_name}.service

%changelog
