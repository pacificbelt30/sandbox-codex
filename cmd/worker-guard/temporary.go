package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
)

func temporaryObservations(targets []string) ([]workerpolicy.TemporaryObservation, error) {
	if len(targets) > 16 {
		return nil, fmt.Errorf("too many temporary filesystems")
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	mounts := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 6 {
			mounts[decode.Replace(fields[4])] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	result := make([]workerpolicy.TemporaryObservation, 0, len(targets))
	for _, target := range targets {
		if !mounts[target] {
			return nil, fmt.Errorf("temporary path is not an exact mountpoint")
		}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(target, &stat); err != nil {
			return nil, err
		}
		info, err := os.Stat(target)
		if err != nil {
			return nil, err
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() {
			return nil, fmt.Errorf("temporary mount is not a directory")
		}
		if stat.Bsize <= 0 || stat.Blocks > uint64((1<<63-1)/stat.Bsize) {
			return nil, fmt.Errorf("temporary capacity overflow")
		}
		result = append(result, workerpolicy.TemporaryObservation{Target: target, Filesystem: int64(stat.Type), MaxBytes: stat.Blocks * uint64(stat.Bsize), UID: owner.Uid, GID: owner.Gid, Mode: uint32(info.Mode().Perm()), Flags: uint64(stat.Flags)})
	}
	return result, nil
}
