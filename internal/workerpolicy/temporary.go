package workerpolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

type StorageMedium string
type StorageLifetime string

const (
	StorageMemory         StorageMedium   = "memory"
	StorageLifetimeVM     StorageLifetime = "vm"
	TemporaryStorageLabel                 = "codex-dock.temporary-storage"
)

// TemporaryStorage is a private, size-limited guest tmpfs. MaxBytes is a
// filesystem ceiling, not reserved memory. Data survives node restart but is
// discarded when the VM stops. Other media/lifetimes fail instead of falling back.
type TemporaryStorage struct {
	Target     string          `json:"target"`
	MaxBytes   int64           `json:"max_bytes"`
	Medium     StorageMedium   `json:"medium"`
	Lifetime   StorageLifetime `json:"lifetime"`
	Executable bool            `json:"executable,omitempty"`
}

func PathsOverlap(a, b string) bool {
	return a == "/" || b == "/" || a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func NormalizeTemporaryStorage(input []TemporaryStorage) ([]TemporaryStorage, error) {
	if len(input) == 0 {
		return nil, nil
	}
	if len(input) > 16 {
		return nil, fmt.Errorf("at most 16 temporary filesystems are supported")
	}
	result := append([]TemporaryStorage(nil), input...)
	for i := range result {
		v := &result[i]
		if v.Medium == "" {
			v.Medium = StorageMemory
		}
		if v.Lifetime == "" {
			v.Lifetime = StorageLifetimeVM
		}
		if !path.IsAbs(v.Target) || path.Clean(v.Target) != v.Target || v.Target == "/" || strings.ContainsAny(v.Target, "\x00\r\n\t") {
			return nil, fmt.Errorf("temporary storage requires a clean absolute guest path")
		}
		if v.Medium != StorageMemory || v.Lifetime != StorageLifetimeVM {
			return nil, fmt.Errorf("temporary storage supports only memory media with VM lifetime")
		}
		if v.MaxBytes < 4096 || v.MaxBytes > 2<<30 || v.MaxBytes%4096 != 0 {
			return nil, fmt.Errorf("temporary size must be page-aligned and within 4 KiB..2 GiB")
		}
		for _, protected := range []string{"/proc", "/sys", "/dev", "/run/codex-dock-guard", "/run/codex-dock-control", "/etc/hosts", "/etc/resolv.conf", "/etc/hostname"} {
			if PathsOverlap(v.Target, protected) {
				return nil, fmt.Errorf("temporary storage overlaps bootstrap or kernel/network paths")
			}
		}
		for _, previous := range result[:i] {
			if PathsOverlap(v.Target, previous.Target) {
				return nil, fmt.Errorf("temporary filesystems cannot overlap")
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Target < result[j].Target })
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if len(data)+len(TemporaryStorageLabel) > 4096 {
		return nil, fmt.Errorf("temporary storage policy exceeds label limit")
	}
	return result, nil
}

func ValidateTemporaryBudget(storage []TemporaryStorage, guestMemory int64) error {
	var total int64
	for _, v := range storage {
		if v.MaxBytes <= 0 || v.MaxBytes > guestMemory-total {
			return fmt.Errorf("temporary filesystem ceilings exceed the guest memory budget")
		}
		total += v.MaxBytes
	}
	return nil
}

func ParseTemporaryStorage(raw string) ([]TemporaryStorage, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw)+len(TemporaryStorageLabel) > 4096 {
		return nil, fmt.Errorf("temporary storage policy exceeds label limit")
	}
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	var storage []TemporaryStorage
	if err := d.Decode(&storage); err != nil {
		return nil, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing temporary storage policy data")
	}
	if len(storage) == 0 {
		return nil, fmt.Errorf("saved temporary storage policy is empty")
	}
	return NormalizeTemporaryStorage(storage)
}

func (v TemporaryStorage) MountOptions() string {
	exec := "noexec"
	if v.Executable {
		exec = "exec"
	}
	return fmt.Sprintf("rw,nosuid,nodev,size=%d,mode=0700,%s", v.MaxBytes, exec)
}

// TemporaryObservation is collected by the pinned guest guard before release.
type TemporaryObservation struct {
	Target     string `json:"target"`
	Filesystem int64  `json:"filesystem"`
	MaxBytes   uint64 `json:"max_bytes"`
	UID        uint32 `json:"uid"`
	GID        uint32 `json:"gid"`
	Mode       uint32 `json:"mode"`
	Flags      uint64 `json:"flags"`
}

func ValidateTemporaryObservation(spec TemporaryStorage, got TemporaryObservation, uid, gid uint32, initial bool) error {
	// Linux statfs flags; verification can also be run by a non-Linux client.
	const tmpfsMagic = 0x01021994
	const readOnly, nosuid, nodev, noexec = uint64(1), uint64(2), uint64(4), uint64(8)
	if got.Target != spec.Target || got.Filesystem != tmpfsMagic || got.MaxBytes != uint64(spec.MaxBytes) || got.UID != uid || got.GID != gid || initial && got.Mode != 0700 || got.Flags&readOnly != 0 || got.Flags&(nosuid|nodev) != (nosuid|nodev) || (got.Flags&noexec == 0) != spec.Executable {
		return fmt.Errorf("temporary filesystem does not match its declared capacity, ownership or mount flags: %s", spec.Target)
	}
	return nil
}
