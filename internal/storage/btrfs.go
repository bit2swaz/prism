package storage

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type BtrfsDriver struct {
	BasePath string // for example, "/mnt/prism_data"
}

func NewBtrfsDriver(basePath string) *BtrfsDriver {
	return &BtrfsDriver{BasePath: basePath}
}

func (d *BtrfsDriver) Init() error {
	info, err := os.Stat(d.BasePath)
	if err != nil {
		return fmt.Errorf("storage path %s does not exist: %w", d.BasePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("storage path %s is not a directory", d.BasePath)
	}
	return nil
}

func (d *BtrfsDriver) CreateSnapshot(sourceID string) (string, error) {
	src := filepath.Join(d.BasePath, sourceID)
	snapName := fmt.Sprintf("snap_%s", sourceID) // simplistic naming for now
	dst := filepath.Join(d.BasePath, snapName)

	cmd := exec.Command("sudo", "btrfs", "subvolume", "snapshot", "-r", src, dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("btrfs snapshot failed: %s: %w", string(output), err)
	}

	return snapName, nil
}

func (d *BtrfsDriver) Clone(snapshotID string, newBranchID string) (string, error) {
	src := filepath.Join(d.BasePath, snapshotID)
	dst := filepath.Join(d.BasePath, newBranchID)

	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}

	cmd := exec.Command("sudo", "btrfs", "subvolume", "snapshot", src, dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("btrfs clone failed: %s: %w", string(output), err)
	}

	return dst, nil
}

func (d *BtrfsDriver) Destroy(id string) error {
	path := filepath.Join(d.BasePath, id)

	cmd := exec.Command("sudo", "btrfs", "subvolume", "delete", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("btrfs delete failed: %s: %w", string(output), err)
	}
	return nil
}
