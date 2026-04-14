FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        live-boot \
        live-config \
        live-config-systemd \
        linux-image-amd64 \
        systemd \
        systemd-sysv \
        ca-certificates \
        nano \
        vim-tiny \
        curl \
        iputils-ping \
        net-tools \
        && rm -rf /var/lib/apt/lists/* \
    && update-initramfs -u -k all

# Networking: systemd-networkd with DHCP on any Ethernet interface (en*/eth*).
RUN mkdir -p /etc/systemd/network \
             /etc/systemd/system/multi-user.target.wants \
             /etc/systemd/system/network-online.target.wants \
    && printf '[Match]\nName=en* eth*\n\n[Network]\nDHCP=yes\n' \
       > /etc/systemd/network/99-dhcp.network \
    && ln -sf /lib/systemd/system/systemd-networkd.service \
              /etc/systemd/system/multi-user.target.wants/systemd-networkd.service \
    && ln -sf /lib/systemd/system/systemd-networkd-wait-online.service \
              /etc/systemd/system/network-online.target.wants/systemd-networkd-wait-online.service

# Root account preparation (autologin is handled by live-config via kernel params).
RUN passwd -d root \
    && chsh -s /bin/bash root

# kompakt agent startup.
COPY files/kompakt-start /usr/local/bin/kompakt-start
COPY files/kompakt-agent.service /lib/systemd/system/kompakt-agent.service
RUN chmod +x /usr/local/bin/kompakt-start \
    && mkdir -p /etc/kompakt /var/lib/kompakt/work \
    && ln -sf /lib/systemd/system/kompakt-agent.service \
              /etc/systemd/system/multi-user.target.wants/kompakt-agent.service
