#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
E2E_CLUSTER_NAME="bgp-e2e"

usage() {
  echo "usage: $(basename "$0") <command>"
  echo ""
  echo "commands:"
  echo "  build   build, vet, and unit test"
  echo "  e2e     run the full e2e suite"
  exit 1
}

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
  echo "=== Pods in bgp-system ==="
  kubectl get pods -n bgp-system -o wide || true
  echo ""
  echo "=== Logs from bgp-system pods ==="
  for POD in $(kubectl get pods -n bgp-system -l app.kubernetes.io/name=bgp -o name 2>/dev/null); do
    echo "--- $POD ---"
    kubectl logs -n bgp-system "$POD" -c bgp --tail=500 2>/dev/null \
      | grep -v "bgp/config: resolved" | tail -50 || true
  done
  echo ""
  echo "=== Events in bgp-system ==="
  kubectl get events -n bgp-system --sort-by='.lastTimestamp' || true
  echo ""
  echo "=== Events in kube-system ==="
  kubectl get events -n kube-system --sort-by='.lastTimestamp' | tail -40 || true
  echo ""
  echo "=== Node status ==="
  kubectl get nodes -o wide || true
  echo ""
  echo "=== BGP resources ==="
  kubectl get bgpinstances,bgpadvertisements,bgpsessions,bgproutepolicies,bgpexternalpeers,bgppeers,bgpproviders \
    --all-namespaces 2>/dev/null || true
  echo ""
  echo "=== VPC resources ==="
  kubectl get vpcs,vpcattachments --all-namespaces 2>/dev/null || true
}

cmd_build() {
  echo "=== Build ==="
  go build ./...

  echo "=== Vet ==="
  go vet ./...

  echo "=== Unit Tests ==="
  go test ./...
}

cmd_e2e() {
  local e2e_dir="${REPO_ROOT}/test/e2e"

  require_tool kind     "go install sigs.k8s.io/kind@latest"
  require_tool kubectl  "https://kubernetes.io/docs/tasks/tools/"
  require_tool task     "https://taskfile.dev/installation/"
  require_tool chainsaw "https://github.com/kyverno/chainsaw/releases/tag/v0.2.12"
  require_tool helm     "https://helm.sh/docs/intro/install/"

  export KUBECONFIG="${e2e_dir}/.kubeconfig"

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
  cd "${e2e_dir}"
  task default
}

case "${1:-}" in
  build) cd "${REPO_ROOT}" && cmd_build ;;
  e2e)   cmd_e2e ;;
  *)     usage ;;
esac
