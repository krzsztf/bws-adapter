# vim: sw=4:ts=4:et


%define relabel_files() \
restorecon -R /var/usrlocal/bin/bws-adapter; \

%define selinux_policyver 41.27-1

Name:   bws_adapter_selinux
Version:	1.0
Release:	1%{?dist}
Summary:	SELinux policy module for bws_adapter

Group:	System Environment/Base
License:	GPLv2+
# This is an example. You will need to change it.
# For a complete guide on packaging your policy
# see https://fedoraproject.org/wiki/SELinux/IndependentPolicy
URL:		http://HOSTNAME
Source0:	bws_adapter.pp
Source1:	bws_adapter.if
Source2:	bws_adapter_selinux.8


Requires: policycoreutils-python-utils, libselinux-utils
Requires(post): selinux-policy-base >= %{selinux_policyver}, policycoreutils-python-utils
Requires(postun): policycoreutils-python-utils
BuildArch: noarch

%description
This package installs and sets up the  SELinux policy security module for bws_adapter.

%install
install -d %{buildroot}%{_datadir}/selinux/packages
install -m 644 %{SOURCE0} %{buildroot}%{_datadir}/selinux/packages
install -d %{buildroot}%{_datadir}/selinux/devel/include/contrib
install -m 644 %{SOURCE1} %{buildroot}%{_datadir}/selinux/devel/include/contrib/
install -d %{buildroot}%{_mandir}/man8/
install -m 644 %{SOURCE2} %{buildroot}%{_mandir}/man8/bws_adapter_selinux.8
install -d %{buildroot}/etc/selinux/targeted/contexts/users/


%post
semodule -n -i %{_datadir}/selinux/packages/bws_adapter.pp

if [ $1 -eq 1 ]; then

fi
if /usr/sbin/selinuxenabled ; then
    /usr/sbin/load_policy
    %relabel_files
fi;
exit 0

%postun
if [ $1 -eq 0 ]; then

    semodule -n -r bws_adapter
    if /usr/sbin/selinuxenabled ; then
       /usr/sbin/load_policy
       %relabel_files
    fi;
fi;
exit 0

%files
%attr(0600,root,root) %{_datadir}/selinux/packages/bws_adapter.pp
%{_datadir}/selinux/devel/include/contrib/bws_adapter.if
%{_mandir}/man8/bws_adapter_selinux.8.*


%changelog
* Tue Dec 31 2024 YOUR NAME <YOUR@EMAILADDRESS> 1.0-1
- Initial version

