#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
E2E_CLUSTER_NAME="bgp-e2e"

require_tool() {
  local tool="$1"
  local hint="$2"
  if ! command -v "${tool}" &>/dev/null; then
    echo "error: '${tool}' not found in PATH" >&2
    echo "  install: ${hint}" >&2
    exit 1
  fi
}

collect_logs() {
  echo "=== Node status ==="
  kubectl get nodes -o wide || true
  echo ""
  echo "=== Events in kube-system ==="
  kubectl get events -n kube-system --sort-by='.lastTimestamp' | tail -40 || true
  echo ""
  echo "=== BGP resources ==="
  kubectl get bgprouters,bgpadvertisements,bgppolicies,bgppeers,bgpvrfinstances \
    --all-namespaces 2>/dev/null || true
  echo ""
  echo "=== VPC resources ==="
  kubectl get vpcs,vpcattachments --all-namespaces 2>/dev/null || true
}

require_tool kind     "go install sigs.k8s.io/kind@latest"
require_tool kubectl  "https://kubernetes.io/docs/tasks/tools/"
require_tool task     "https://taskfile.dev/installation/"
require_tool chainsaw "https://github.com/kyverno/chainsaw/releases/tag/v0.2.12"
require_tool helm     "https://helm.sh/docs/intro/install/"

E2E_DIR="${REPO_ROOT}/test/e2e"
export KUBECONFIG="${E2E_DIR}/.kubeconfig"

on_exit() {
  local exit_code=$?
  if [ "${exit_code}" -ne 0 ]; then
    echo "=== E2E failed — collecting diagnostic logs ==="
    collect_logs
  fi
  echo "=== Deleting kind cluster ==="
  kind delete cluster --name "${E2E_CLUSTER_NAME}" || true
  exit "${exit_code}"
}
trap on_exit EXIT

echo "=== Running E2E suite ==="
cd "${E2E_DIR}"
task default
