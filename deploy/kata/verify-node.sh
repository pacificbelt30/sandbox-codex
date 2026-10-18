#!/bin/bash
# Root controller inside an explicitly disposable, network=none setup test node.
set -euo pipefail
[[ -f /.dockerenv && ${CODEX_DOCK_DISPOSABLE_NODE:-} == 1 ]]
proof() { echo "VERIFIED: $*"; }
operator() { runuser -u kata-test -- env HOME=/home/kata-test OPENAI_API_KEY=sk-setup-fixture-never-valid "$@"; }
release() { python3 -c 'import json; print(json.load(open("/var/lib/codex-dock-setup/installed.json"))["release"])'; }
cli() { operator /usr/local/bin/codex-dock "$@"; }
ctr_node() { "$(release)/bin/ctr" --address /run/codex-dock-containerd/containerd.sock --namespace codex-dock "$@"; }
mkdir -p /evidence /var/cache/codex-dock-setup
for archive in /cache-input/*; do cp "$archive" /var/cache/codex-dock-setup/; done
chmod -R go-w /var/cache/codex-dock-setup
/bundle/codex-dock setup --bundle /bundle --operator kata-test --offline > /evidence/plan.txt
[[ ! -e /etc/systemd/system/codex-dock-node.service ]]
proof 'planning leaves services uninstalled'
/bundle/codex-dock setup --bundle /bundle --operator kata-test --offline --apply > /evidence/setup.txt 2>&1
cli runtime-check --json > /evidence/readiness.json
initial_release=$(release)
proof 'fresh setup and operator readiness'
ctr_node images import --local /images.tar > /evidence/import.txt
install -d -o kata-test -g 1000 -m 0700 /home/kata-test/.config/codex-dock /home/kata-test/project
proxy_fixture=${CODEX_DOCK_KATA_PROXY_IMAGE:-codex-dock-proxy:policy-verification}
export CODEX_DOCK_KATA_PROXY_IMAGE=$proxy_fixture
python3 - <<'PYCONFIG'
import json,os
from pathlib import Path
Path('/home/kata-test/.config/codex-dock/config.toml').write_text('default_image = "codex-dock:kata-guard"\nproxy_image = '+json.dumps(os.environ['CODEX_DOCK_KATA_PROXY_IMAGE'])+'\nauto_build = false\n')
PYCONFIG
chown kata-test:1000 /home/kata-test/.config/codex-dock/config.toml
chmod 0600 /home/kata-test/.config/codex-dock/config.toml
cli init > /evidence/init.txt 2>&1
cli image check codex-dock:kata-guard --agent codex --json > /evidence/image.json
proof 'init prepares proxy and validates custom-compatible image without real credentials'
cd /home/kata-test/project
printf host > input
chown kata-test:1000 input
cli run --name setup-worker --detach --keep --no-internet --watch-polling --exec 'python3 -u -c '\''import os,time;from pathlib import Path;assert os.environ["CHOKIDAR_USEPOLLING"]=="true";assert Path("/workspace/input").read_text()=="host";Path("/workspace/created").write_text(f"{os.getuid()}:{os.getgid()}");time.sleep(600)'\''' > /evidence/worker.txt 2>&1
for attempt in $(seq 1 300); do [[ -f created ]] && break; sleep .2; done
if [[ ! -f created ]]; then
 cli logs setup-worker > /evidence/worker-logs.txt 2>&1 || true
 cli ps -a > /evidence/containers-at-timeout.txt 2>&1 || true
 cli vm inspect setup-worker > /evidence/worker-timeout.json 2>&1 || true
 for hosts in /var/lib/codex-dock-node/engine/tasks/*/hosts; do
  [[ -f $hosts ]] || continue
  if grep -q codex-auth-proxy "$hosts"; then
   cp "$hosts" /evidence/worker-hosts-at-timeout.txt
   break
  fi
 done
 {
  ip -brief link
  ip -brief address
  bridge link
  ip netns list
  for namespace in $(ip netns list | awk '{print $1}'); do
   echo "namespace: $namespace"
   ip -n "$namespace" -brief address
   ip -n "$namespace" route
   ip -n "$namespace" neigh
  done
  for interface in /sys/class/net/v*/ifindex; do
   [[ -e $interface ]] || continue
   name=${interface%/ifindex}
   name=${name##*/}
   echo "tc ingress: $name"
   tc filter show dev "$name" ingress
  done
 } > /evidence/network-at-timeout.txt 2>&1 || true
 echo 'worker did not create the shared file within 60 seconds' >&2
 exit 1
fi
[[ $(cat created) == 1000:1000 && $(stat -c '%u:%g' created) == 1000:1000 ]]
cli vm ps -a --json > /evidence/vms-before-refusal.json
if /bundle-next/codex-dock setup --bundle /bundle-next --operator kata-test --offline --apply > /evidence/refused-update.txt 2>&1; then
 echo 'update accepted a running worker' >&2; exit 1
fi
[[ $(release) == "$initial_release" ]]
cli vm inspect setup-worker > /evidence/worker-after-refusal.json
cli runtime-check --json > /evidence/readiness-after-refusal.json
proof 'live workspace, numeric ownership, polling environment and refusal to update live resources'
cli rm --force setup-worker
cli proxy stop
cli proxy rm
cli network rm
if [[ -x /acceptance.test ]]; then
 install -d -o kata-test -g 1000 -m 0700 /home/kata-test/owner-fixture
 install -d -o 12001 -g 12002 -m 0777 /home/kata-test/owner-fixture/project
 install -o 12001 -g 12002 -m 0600 /dev/null /home/kata-test/owner-fixture/project/foreign-private
 install -d -o 12001 -g 12002 -m 2777 /home/kata-test/owner-fixture/project/setgid
 install -d -o root -g 1000 -m 0770 /run/setup-node-restart
 operator env CODEX_DOCK_KATA_E2E=1 CODEX_DOCK_KATA_TEST_BINARY=/usr/local/bin/codex-dock \
  CODEX_DOCK_KATA_TEST_IMAGE=codex-dock:kata-guard CODEX_DOCK_KATA_PROXY_IMAGE="$proxy_fixture" \
  CODEX_DOCK_KATA_OWNER_FIXTURE=/home/kata-test/owner-fixture/project \
  CODEX_DOCK_NODE_RESTART_REQUEST_DIR=/run/setup-node-restart \
  /acceptance.test -test.run '^TestKata(Acceptance|WorkspaceSharing)$' -test.v -test.count=1 -test.timeout=15m > /evidence/acceptance.txt 2>&1 &
 test_pid=$!
 while kill -0 "$test_pid" 2>/dev/null; do
  if [[ -f /run/setup-node-restart/request ]]; then
   read -r action cid < /run/setup-node-restart/request
   rm /run/setup-node-restart/request
   if [[ $action == namespace ]]; then
    [[ $cid =~ ^[0-9a-f]{64}$ ]]
    [[ -z $(ctr_node tasks list -q | awk -v cid="$cid" '$0==cid{print}') ]]
    ip netns del "cdx-${cid:0:24}"
   else
    [[ $action == restart ]]
    systemctl restart codex-dock-node
    for attempt in $(seq 1 60); do [[ -S /run/codex-dock-node.sock ]] && break; sleep .2; done
   fi
   printf complete > /run/setup-node-restart/complete
  fi
  sleep .2
 done
 wait "$test_pid"
 proof 'full native Kata lifecycle and workspace acceptance on newly installed node'
fi
ctr_node images list -q | sort > /evidence/images-before-update.txt
/bundle-next/codex-dock setup --bundle /bundle-next --operator kata-test --offline --apply > /evidence/update.txt 2>&1
updated_release=$(release)
[[ $updated_release != "$initial_release" ]]
cli runtime-check --json > /evidence/readiness-updated.json
ctr_node images list -q | sort > /evidence/images-after-update.txt
cmp /evidence/images-before-update.txt /evidence/images-after-update.txt
proof 'successful matched-version update retains images and project files'
# Fail only the newly staged BuildKit executable. Old release assets stay intact.
/bundle/codex-dock setup --bundle /bundle --operator kata-test --offline --apply > /evidence/failed-update.txt 2>&1 &
update_pid=$!
faulted=''
for attempt in $(seq 1 3000); do
 for candidate in /opt/codex-dock/releases/*/bin/buildkitd; do
  if [[ -x $candidate && $candidate != "$updated_release/bin/buildkitd" && $candidate != "$initial_release/bin/buildkitd" ]]; then
   chmod 000 "$candidate"
   faulted=$candidate
   break
  fi
 done
 [[ -n $faulted ]] && break
 kill -0 "$update_pid" 2>/dev/null || break
 sleep .1
done
[[ -n $faulted ]]
if wait "$update_pid"; then echo 'faulted update unexpectedly succeeded' >&2; exit 1; fi
chmod 0755 "$faulted"
[[ $(release) == "$updated_release" ]]
cli runtime-check --json > /evidence/readiness-after-failure.json
[[ $(cat created) == 1000:1000 ]]
proof 'failed new service activation automatically restores previous working release'
/usr/local/bin/codex-dock setup --rollback --apply > /evidence/rollback.txt 2>&1
[[ $(release) == "$initial_release" ]]
cli runtime-check --json > /evidence/readiness-rollback.json
cli vm ps -a --json > /evidence/final-vms.json
[[ -z $(ctr_node containers list -q) && -z $(ctr_node tasks list -q) ]]
[[ -z $(ip netns list) ]]
[[ -z $(nft list tables | awk '$3 ~ /^cdx_/ { print }') ]]
proof 'explicit rollback restores matching CLI/node with no remaining containers, VMs or networks'
