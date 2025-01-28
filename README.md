# Expose Bitwarden Secret Manager (BWS) secrets as systemd credentials

This is hobby project, DO NOT assume its secure enough for production workloads.

## Installation

bws-adapter is designed to run as system service, so installation must be done by root.

Store BWS access token as systemd encrypted secret:

    mkdir -p /etc/bws-adapter
    systemd-ask-password -n | systemd-creds encrypt --name=bws-access-token - /etc/bws-adapter/bws-access-token.cred
    
Load SELinux policy:

    ./config/selinux/bws_adapter.sh
    
Install as systemd unit:

    cp config/systemd/bws-adapter.container /etc/containers/systemd
    systemctl daemon-reload
    systemctl enable --now bws-adapter.service
    
## Usage

In your service unit files, load credentials from /run/bws/bws.sock Unix socket:

    LoadCredential=some-secret:/run/bws/bws.sock
    LoadCredential=other-secret:/run/bws/bws.sock
    
bws-adapter will query Bitwarden Secret Manager for secrets, and they
will be exposed to your service inside as credentials.



