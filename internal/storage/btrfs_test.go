package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBtrfsWorkflow(t *testing.T) {
	basePath := "/mnt/prism_data"
	driver := NewBtrfsDriver(basePath)

	masterPath := filepath.Join(basePath, "master")
	if _, err := os.Stat(masterPath); os.IsNotExist(err) {
		t.Fatalf("Pre-requisite missing: %s does not exist. Run 'sudo btrfs subvolume create %s'", masterPath, masterPath)
	}

	snapID, err := driver.CreateSnapshot("master")
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	t.Logf("Snapshot created: %s", snapID)

	branchName := "test_branch_go"
	mountPath, err := driver.Clone(snapID, branchName)
	if err != nil {
		t.Fatalf("Clone failed: %v", err)
	}
	t.Logf("Branch cloned to: %s", mountPath)

	if _, err := os.Stat(mountPath); os.IsNotExist(err) {
		t.Errorf("Cloned path does not exist on disk")
	}

	if err := driver.Destroy(branchName); err != nil {
		t.Errorf("Cleanup branch failed: %v", err)
	}
	if err := driver.Destroy(snapID); err != nil {
		t.Errorf("Cleanup snapshot failed: %v", err)
	}
}
