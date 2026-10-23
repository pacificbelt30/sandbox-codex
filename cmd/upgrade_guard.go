package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func rejectUnsupportedProfileDowngrade(tag, directory string) error {
	version := strings.SplitN(strings.TrimPrefix(tag, "v"), "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return fmt.Errorf("invalid release version")
	}
	var numbers [3]uint64
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid release version")
		}
		numbers[i] = n
	}
	if numbers[0] != 0 || numbers[1] != 0 || numbers[2] > 2 {
		return nil
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		_, err := os.Lstat(filepath.Join(directory, entry.Name(), entry.Name()+".vault"))
		if err == nil {
			return fmt.Errorf("release %s cannot read encrypted profiles; retain a compatible CLI and reauthenticate separately instead of downgrading this installation", tag)
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
