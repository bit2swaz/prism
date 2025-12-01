package storage

type Driver interface {
	Init() error

	CreateSnapshot(sourceID string) (string, error)

	Clone(snapshotID string, newBranchID string) (string, error)

	Destroy(id string) error
}
