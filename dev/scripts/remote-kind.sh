#!/usr/bin/env bash
# Run once on a Debian/Ubuntu host with Tailscale. Write the kubeconfig to
# stdout and setup logs to stderr.
set -euo pipefail

{
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y docker.io golang-go ca-certificates
    systemctl enable --now docker
    GOBIN=/usr/local/bin go install sigs.k8s.io/kind@v0.33.0

    kind create cluster --name remote --config=- <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  apiServerAddress: "0.0.0.0"
EOF

    docker run -d --name kind-registry --network kind \
        -p 127.0.0.1:5005:5000 registry:3

    # Node-local localhost must resolve to the registry container for image pulls.
    docker exec remote-control-plane mkdir -p /etc/containerd/certs.d/localhost:5005
    echo '[host."http://kind-registry:5000"]' |
        docker exec -i remote-control-plane cp /dev/stdin /etc/containerd/certs.d/localhost:5005/hosts.toml

} >&2

kind get kubeconfig --name remote
