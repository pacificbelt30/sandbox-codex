package sandbox

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/pacificbelt30/codex-dock/internal/workerpolicy"
)

func validateTemporaryTopology(storage []workerpolicy.TemporaryStorage, binds []mount.Mount, denied []string, readOnlyRoot bool) error {
	for _, v := range storage {
		for _, b := range binds {
			target := path.Clean(b.Target)
			covers := v.Target == target || strings.HasPrefix(target, v.Target+"/")
			if covers || b.ReadOnly && workerpolicy.PathsOverlap(v.Target, target) {
				return fmt.Errorf("temporary filesystem would hide a bind mount or bypass read-only access")
			}
		}
		for _, target := range denied {
			if workerpolicy.PathsOverlap(v.Target, target) {
				return fmt.Errorf("temporary filesystem overlaps a filesystem restriction")
			}
		}
		if readOnlyRoot {
			for _, target := range []string{"/tmp", "/var/tmp"} {
				if v.Target != target && strings.HasPrefix(target, v.Target+"/") {
					return fmt.Errorf("temporary ceiling cannot contain another default tmpfs")
				}
			}
		}
	}
	return nil
}

func validateSavedTemporaryStorage(v container.InspectResponse, guestMemory int64) ([]workerpolicy.TemporaryStorage, error) {
	storage, err := workerpolicy.ParseTemporaryStorage(v.Config.Labels[workerpolicy.TemporaryStorageLabel])
	if err != nil {
		return nil, err
	}
	if len(storage) == 0 {
		return nil, nil
	}
	if err := workerpolicy.ValidateTemporaryBudget(storage, guestMemory); err != nil {
		return nil, err
	}
	denied := append(append([]string(nil), v.HostConfig.MaskedPaths...), v.HostConfig.ReadonlyPaths...)
	if err := validateTemporaryTopology(storage, v.HostConfig.Mounts, denied, v.HostConfig.ReadonlyRootfs); err != nil {
		return nil, err
	}
	allowed := map[string]bool{"/run/codex-dock-control": true}
	if v.HostConfig.ReadonlyRootfs {
		allowed["/tmp"] = true
		allowed["/var/tmp"] = true
	}
	for _, spec := range storage {
		allowed[spec.Target] = true
		if v.HostConfig.Tmpfs[spec.Target] != spec.MountOptions() {
			return nil, fmt.Errorf("saved temporary filesystem settings changed")
		}
	}
	for target := range v.HostConfig.Tmpfs {
		if !allowed[target] {
			return nil, fmt.Errorf("untracked temporary filesystem in retained worker")
		}
	}
	return storage, nil
}

func verifyTemporaryGuest(data []byte, storage []workerpolicy.TemporaryStorage, initial bool) error {
	if len(storage) == 0 {
		return nil
	}
	var probe map[string]string
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	var observations []workerpolicy.TemporaryObservation
	if err := json.Unmarshal([]byte(probe["temporary_storage"]), &observations); err != nil {
		return fmt.Errorf("guest bootstrap does not report temporary filesystems: %w", err)
	}
	if len(observations) != len(storage) {
		return fmt.Errorf("temporary filesystem observation count mismatch")
	}
	var uid, gid uint64
	var haveUID, haveGID bool
	for _, line := range strings.Split(probe["status"], "\n") {
		name, raw, ok := strings.Cut(line, ":")
		values := strings.Fields(raw)
		if !ok || len(values) != 4 {
			continue
		}
		switch name {
		case "Uid":
			uid, _ = strconv.ParseUint(values[0], 10, 32)
			haveUID = uid != 0
		case "Gid":
			var err error
			gid, err = strconv.ParseUint(values[0], 10, 32)
			haveGID = err == nil
		}
	}
	if !haveUID || !haveGID {
		return fmt.Errorf("guest temporary owner could not be verified")
	}
	seen := make(map[string]workerpolicy.TemporaryObservation)
	for _, o := range observations {
		if _, exists := seen[o.Target]; exists {
			return fmt.Errorf("duplicate temporary filesystem observation")
		}
		seen[o.Target] = o
	}
	for _, spec := range storage {
		if err := workerpolicy.ValidateTemporaryObservation(spec, seen[spec.Target], uint32(uid), uint32(gid), initial); err != nil {
			return err
		}
	}
	return nil
}
