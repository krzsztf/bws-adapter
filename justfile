fmt:
    go fmt

vet: fmt
    go vet

build: vet
    rm -f bws-adapter
    # https://github.com/bitwarden/sdk-go/pull/2
    # https://github.com/bitwarden/sdk-sm/pull/1120
    go build --ldflags '-extldflags "-lm"'

run: build
    #!/bin/bash
    rm -f /run/user/1000/bws.socket
    BWS_ACCESS_TOKEN=$(secret-tool lookup id corebox-bws-token) ./bws-adapter

install: build
    cp systemd/bws-adapter.service ~/.config/systemd
    systemctl --user daemon-reload
    systemctl --user start bws-adapter.service
    systemctl --user status bws-adapter.service
    # journalctl --user -u bws-adapter -n 100
    
test:
    systemd-run --user -P -p LoadCredential=corebox-pg-linkding-password:/run/user/1000/bws.socket -p LoadCredential=maxmind-license-key:/run/user/1000/bws.socket ./print-creds.sh
