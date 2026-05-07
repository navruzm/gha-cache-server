package db

type CacheEntry struct {
	ID         string
	Key        string
	Version    string
	Scope      string
	RepoID     string
	UpdatedAt  int64
	LocationID string
}

type StorageLocation struct {
	ID               string
	FolderName       string
	PartCount        int
	MergeStartedAt   *int64
	MergedAt         *int64
	PartsDeletedAt   *int64
	LastDownloadedAt *int64
}

type Upload struct {
	ID                      int64
	Key                     string
	Version                 string
	Scope                   string
	RepoID                  string
	CreatedAt               int64
	LastPartUploadedAt      *int64
	StartedPartUploadCount  int
	FinishedPartUploadCount int
	FolderName              string
}
