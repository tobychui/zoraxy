# Packages a prebuilt static Zoraxy binary. Build it with packaging/linux/Makefile:
#   make -C packaging/linux rpm [ARCH=amd64|arm64|arm|386|riscv64]
# The Makefile passes the version, release and source directory.

%global debug_package %{nil}
%global __os_install_post %{nil}
%define _build_id_links none
# xz payload: readable by rpm 4.14 (EL8) as well as newer releases
%define _binary_payload w9.xzdio

%{!?zoraxy_version: %global zoraxy_version 0.0.0}
%{!?zoraxy_release: %global zoraxy_release 1}

Name:           zoraxy
Version:        %{zoraxy_version}
Release:        %{zoraxy_release}
Summary:        General purpose HTTP reverse proxy and forwarding tool
License:        AGPL-3.0-only
URL:            https://github.com/tobychui/zoraxy
Source0:        zoraxy
Source1:        zoraxy.service
Source2:        zoraxy.default
Source3:        LICENSE
AutoReqProv:    no
Requires(pre):  shadow-utils

%description
Zoraxy is a general purpose HTTP reverse proxy and forwarding tool with a web
management interface: TLS/ACME certificates, access control, load balancing,
stream (TCP/UDP) proxy, statistics and plugins.

The service runs as the "zoraxy" user and keeps its data in /var/lib/zoraxy.
Options are set in /etc/sysconfig/zoraxy.

%install
install -D -m 0755 %{SOURCE0} %{buildroot}/usr/bin/zoraxy
install -D -m 0644 %{SOURCE1} %{buildroot}/usr/lib/systemd/system/zoraxy.service
install -D -m 0644 %{SOURCE2} %{buildroot}/etc/sysconfig/zoraxy
install -D -m 0644 %{SOURCE3} %{buildroot}/usr/share/licenses/zoraxy/LICENSE
install -d -m 0750 %{buildroot}/var/lib/zoraxy

%pre
getent group zoraxy >/dev/null || groupadd -r zoraxy
getent passwd zoraxy >/dev/null || \
    useradd -r -g zoraxy -d /var/lib/zoraxy -s /sbin/nologin -c "Zoraxy reverse proxy" zoraxy
exit 0

%post
systemctl daemon-reload >/dev/null 2>&1 || :
if [ $1 -eq 1 ]; then
    echo "Zoraxy installed. Review /etc/sysconfig/zoraxy, then: systemctl enable --now zoraxy"
fi

%preun
if [ $1 -eq 0 ]; then
    systemctl --no-reload disable --now zoraxy.service >/dev/null 2>&1 || :
fi

%postun
systemctl daemon-reload >/dev/null 2>&1 || :
if [ $1 -ge 1 ]; then
    systemctl try-restart zoraxy.service >/dev/null 2>&1 || :
fi

%files
%license /usr/share/licenses/zoraxy/LICENSE
/usr/bin/zoraxy
/usr/lib/systemd/system/zoraxy.service
%config(noreplace) /etc/sysconfig/zoraxy
%dir %attr(0750,zoraxy,zoraxy) /var/lib/zoraxy
