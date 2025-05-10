#!/bin/bash

# Ensure this script is run with root privileges
if [ "$EUID" -ne 0 ]; then
  echo "Please run as root"
  exit 1
fi

HOSTNAME=$(hostname)

if [ "$HOSTNAME" = "controller-node" ]; then
  # Apply Flannel CNI plugin
  kubectl apply -f https://raw.githubusercontent.com/flannel-io/flannel/master/Documentation/kube-flannel.yml

  # Install checkpoint-access Helm chart
  helm install checkpoint-access ./container-live-migration/k8s-checkpoint-access \
    --namespace kube-system --create-namespace

  # Create a Kubernetes token
  K8S_TOKEN=$(kubectl -n kube-system create token checkpoint-sa --duration=3h)
  echo "K8S_TOKEN created: $K8S_TOKEN"

else
  # Mount NFS volume
  mount -t nfs 192.168.122.101:/mnt/nfs /mnt/nfs
fi
