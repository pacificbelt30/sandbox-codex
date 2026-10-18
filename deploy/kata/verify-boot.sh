#!/bin/sh
# Only for the disposable private PID/mount/cgroup namespaces created by verify-setup.py.
set -eu
test -f /.dockerenv
test "${CODEX_DOCK_DISPOSABLE_NODE:-}" = 1
mount --make-rprivate /
mount --bind /proc/sys /proc/sys
mount -o remount,rw,bind /proc/sys /proc/sys
umount /sys/fs/cgroup
mount -t cgroup2 none /sys/fs/cgroup
exec /sbin/init
