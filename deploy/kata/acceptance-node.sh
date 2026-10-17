#!/bin/sh
set -eu
# Run only inside an explicitly disposable, network=none Linux/KVM test node.
# Inputs: /kata-input (runtime-rs assets), /guest-input (prepared guest),
# /work/{kata.toml,codex-dock,codex-dock-io-logger,worker-guard,sandbox.test}.
# Preload images via /images.tar or /work/images.tar. An optional /work tool
# bundle supports the minimal Alpine fixture; a full Linux node can use its
# installed containerd/ctr/iproute2/nftables/runc/buildctl/buildkitd instead.
# No host Docker socket or host PID namespace may be passed to this node.
test -f /.dockerenv
test "${CODEX_DOCK_DISPOSABLE_NODE:-}" = 1
mount --make-rprivate /
mount --bind /proc/sys /proc/sys
mount -o remount,rw,bind /proc/sys /proc/sys
umount /sys/fs/cgroup
mount -t cgroup2 none /sys/fs/cgroup
mkdir -p /sys/fs/cgroup/supervisor
for pid in $(cat /sys/fs/cgroup/cgroup.procs); do
 if ! printf '%s\n' "$pid" > /sys/fs/cgroup/supervisor/cgroup.procs; then
  if kill -0 "$pid" 2>/dev/null; then exit 1; fi
 fi
done
printf '+cpu +memory +pids\n' > /sys/fs/cgroup/cgroup.subtree_control
mkdir -p /opt/kata /opt/guest /opt/codex-dock-kata /usr/local/bin /etc/codex-dock
cp -r /kata-input/. /opt/kata/
cp -r /guest-input/. /opt/guest/
cp -r /guest-input/. /opt/codex-dock-kata/
if [ -d /work/glibc ]; then
 mkdir -p /lib64 /usr/lib/x86_64-linux-gnu
 cp /work/glibc/*.so* /usr/lib/x86_64-linux-gnu/
 cp /work/glibc/ld-linux-x86-64.so.2 /lib64/
fi
if [ -f /work/containerd ]; then cp /work/containerd /work/ctr /usr/local/bin/; fi
if [ -d /work/host-tools ]; then cp /work/host-tools/* /usr/local/bin/; fi
cp /work/kata.toml /etc/codex-dock/kata.toml
chown -R 0:0 /opt/kata /opt/guest /opt/codex-dock-kata /etc/codex-dock /usr/local/bin
chmod -R go-w /opt/kata /opt/guest /opt/codex-dock-kata /etc/codex-dock /usr/local/bin
cat > /etc/codex-dock/containerd.toml <<'TOML'
version = 3
root = "/data"
state = "/run/codex-dock-containerd"
disabled_plugins = ["io.containerd.cri.v1.images", "io.containerd.cri.v1.runtime"]
[grpc]
address = "/run/codex-dock-containerd/containerd.sock"
TOML
containerd --config /etc/codex-dock/containerd.toml > /work/containerd.log 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true' EXIT INT TERM
export CONTAINERD_ADDRESS=/run/codex-dock-containerd/containerd.sock
export CONTAINERD_NAMESPACE=codex-dock
for attempt in $(seq 1 60); do
 if ctr version > /dev/null 2>&1; then break; fi
 sleep 1
done
if ! ctr images inspect docker.io/library/codex-dock:kata-guard >/dev/null 2>&1; then
 image_archive=/images.tar
 if [ ! -r "$image_archive" ]; then image_archive=/work/images.tar; fi
 ctr images import --local "$image_archive" > /work/import.log
fi
mkdir -p /opt/codex-dock /sys/fs/cgroup/codex-dock /run/codex-dock-buildkit
printf '+cpu +memory +pids\n' > /sys/fs/cgroup/codex-dock/cgroup.subtree_control
cp /work/codex-dock /usr/local/bin/codex-dock
cp /work/codex-dock-io-logger /usr/local/bin/codex-dock-io-logger
cp /work/worker-guard /opt/codex-dock/worker-guard
if [ -d /work/buildkit/bin ]; then cp /work/buildkit/bin/* /usr/local/bin/; fi
chown -R 0:0 /opt/codex-dock /usr/local/bin
chmod -R go-w /opt/codex-dock /usr/local/bin
sysctl -q -w net.ipv4.ip_forward=1
if ! id -u kata-test >/dev/null 2>&1; then
 addgroup -g 1000 kata-test
 adduser -D -u 1000 -G kata-test kata-test
fi
# Test evidence is an input-only host mount for root's service binaries.
rm -f /etc/codex-dock/kata-assets.json
/usr/local/bin/codex-dock runtime-lock --kata-config /etc/codex-dock/kata.toml --daemon-config /etc/codex-dock/containerd.toml --containerd /usr/local/bin/containerd --runtime-shim /opt/kata/runtime-rs/bin/containerd-shim-kata-v2 --launcher /opt/codex-dock/worker-guard
cat > /etc/codex-dock/buildkit.toml <<'TOML'
root = "/data/buildkit"
[grpc]
 address = ["unix:///run/codex-dock-buildkit/buildkitd.sock"]
 uid = 0
 gid = 1000
[worker.oci]
 enabled = false
[worker.containerd]
 enabled = true
 address = "/run/codex-dock-containerd/containerd.sock"
 namespace = "codex-dock"
 snapshotter = "overlayfs"
TOML
buildkitd --config /etc/codex-dock/buildkit.toml > /work/buildkit.log 2>&1 &
buildkit_pid=$!
/usr/local/bin/codex-dock node serve --group kata-test > /work/node.log 2>&1 &
node_pid=$!
trap 'kill "$node_pid" "$buildkit_pid" "$pid" 2>/dev/null || true; wait "$node_pid" "$buildkit_pid" "$pid" 2>/dev/null || true' EXIT INT TERM
for attempt in $(seq 1 60); do
 if [ -S /run/codex-dock-node.sock ] && [ -S /run/codex-dock-buildkit/buildkitd.sock ]; then break; fi
 sleep 1
done
export CODEX_DOCK_KATA_E2E=1
export CODEX_DOCK_KATA_TEST_BINARY=/work/codex-dock
export CODEX_DOCK_KATA_TEST_IMAGE="${CODEX_DOCK_KATA_TEST_IMAGE:-codex-dock:kata-guard}"
export CODEX_DOCK_KATA_PROXY_IMAGE="${CODEX_DOCK_KATA_PROXY_IMAGE:-codex-dock-proxy:policy-verification}"
mkdir -p /tmp/codex-dock-node-restart
rm -f /tmp/codex-dock-node-restart/request /tmp/codex-dock-node-restart/complete
chown 0:1000 /tmp/codex-dock-node-restart
chmod 0770 /tmp/codex-dock-node-restart
export CODEX_DOCK_NODE_RESTART_REQUEST_DIR=/tmp/codex-dock-node-restart
su -s /bin/sh kata-test -c '/work/sandbox.test -test.run "^TestKata(Acceptance|WorkspaceSharing)$" -test.count=1 -test.v -test.timeout=15m' &
test_pid=$!
while kill -0 "$test_pid" 2>/dev/null; do
 if [ -f /tmp/codex-dock-node-restart/request ]; then
  read -r operation request_cid < /tmp/codex-dock-node-restart/request
  rm /tmp/codex-dock-node-restart/request
  if [ "$operation" = namespace ]; then
   case "$request_cid" in *[!0-9a-f]*|"") exit 1;; esac
   test "${#request_cid}" = 64
   ctr tasks ls > /tmp/recovery-task-list
   if awk -v cid="$request_cid" '$1==cid {found=1} END {exit !found}' /tmp/recovery-task-list; then exit 1; fi
   ip netns del "cdx-$(printf '%s' "$request_cid" | cut -c1-24)"
  else
   test "$operation" = restart
  fi
  kill "$node_pid"
  wait "$node_pid"
  /usr/local/bin/codex-dock node serve --group kata-test >> /work/node.log 2>&1 &
  node_pid=$!
  for attempt in $(seq 1 60); do
   if [ -S /run/codex-dock-node.sock ]; then break; fi
   sleep 1
  done
  touch /tmp/codex-dock-node-restart/complete
 fi
 sleep 0.2
done
wait "$test_pid"
