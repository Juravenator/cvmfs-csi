#!/usr/bin/env bash
set -o errexit -o nounset -o pipefail
IFS=$'\n\t\v'
cd `dirname "${BASH_SOURCE[0]:-$0}"`

kubectl apply -f pod-with-cvmfs.yaml
kubectl wait --for=condition=ready pod cvmfs-example
kubectl exec -it cvmfs-example -- bash