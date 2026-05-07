package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	dbpkg "github.com/navruzm/github-actions-cache-server-go/internal/db"
	"github.com/navruzm/github-actions-cache-server-go/internal/ids"
)

type ServiceConfig struct {
	APIBaseURL            string
	EnableDirectDownloads bool
	Logger                *slog.Logger
}

type Service struct {
	q       *dbpkg.Queries
	adapter Adapter
	cfg     ServiceConfig

	mergesMu sync.Mutex
	merges   map[string]chan struct{}
}

func NewService(q *dbpkg.Queries, a Adapter, cfg ServiceConfig) *Service {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Service{q: q, adapter: a, cfg: cfg, merges: map[string]chan struct{}{}}
}

func (s *Service) Adapter() Adapter { return s.adapter }

func (s *Service) WaitForOngoingMerges(ctx context.Context) {
	s.mergesMu.Lock()
	chans := make([]chan struct{}, 0, len(s.merges))
	for _, c := range s.merges {
		chans = append(chans, c)
	}
	s.mergesMu.Unlock()
	for _, c := range chans {
		select {
		case <-c:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) CreateUpload(ctx context.Context, key, version, scope, repoID string) (*dbpkg.Upload, error) {
	existing, err := s.q.FindUploadByKey(ctx, key, version, scope, repoID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, nil
	}
	id := ids.NumericID()
	u := dbpkg.Upload{
		ID:         id,
		Key:        key,
		Version:    version,
		Scope:      scope,
		RepoID:     repoID,
		CreatedAt:  time.Now().UnixMilli(),
		FolderName: strconv.FormatInt(id, 10),
	}
	if err := s.q.InsertUpload(ctx, u); err != nil {
		return nil, fmt.Errorf("insert upload: %w", err)
	}
	return &u, nil
}

var ErrInvalidUpload = errors.New("invalid upload")

func (s *Service) UploadPart(ctx context.Context, uploadID int64, partIndex int, body io.Reader) error {
	u, err := s.q.FindUploadByID(ctx, uploadID)
	if err != nil {
		return err
	}
	if u == nil {
		return nil
	}
	if err := s.q.IncStartedPartCount(ctx, uploadID); err != nil {
		return err
	}
	if err := s.adapter.UploadStream(ctx, fmt.Sprintf("%s/parts/%d", u.FolderName, partIndex), body); err != nil {
		return err
	}
	return s.q.IncFinishedPartCount(ctx, uploadID, time.Now().UnixMilli())
}

func (s *Service) CompleteUpload(ctx context.Context, key, version, scope, repoID string) (*dbpkg.Upload, error) {
	u, err := s.q.FindUploadByKey(ctx, key, version, scope, repoID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	if u.FinishedPartUploadCount == 0 {
		_ = s.q.DeleteUpload(ctx, u.ID)
		return nil, errors.New("no parts have been uploaded")
	}
	if u.StartedPartUploadCount != u.FinishedPartUploadCount {
		_ = s.q.DeleteUpload(ctx, u.ID)
		return nil, fmt.Errorf("not all parts uploaded (%d of %d)", u.FinishedPartUploadCount, u.StartedPartUploadCount)
	}

	partsFolder := fmt.Sprintf("%s/parts", u.FolderName)
	partCount, err := s.adapter.CountFilesInFolder(ctx, partsFolder)
	if err != nil {
		return nil, fmt.Errorf("count parts: %w", err)
	}
	if partCount != u.FinishedPartUploadCount {
		_ = s.q.DeleteUpload(ctx, u.ID)
		return nil, fmt.Errorf("part count mismatch: db=%d storage=%d", u.FinishedPartUploadCount, partCount)
	}

	// NOTE: Queries currently execute against the underlying *sql.DB (not a *sql.Tx),
	// so a BeginTx wrapper here would not provide atomicity AND would deadlock SQLite
	// tests/runtime configured with SetMaxOpenConns(1) (the begun tx pins the only
	// connection). The TS upstream has the same adapter/DB partial-failure window.
	locID := ids.UUIDv4()
	if err := s.q.InsertStorageLocation(ctx, dbpkg.StorageLocation{
		ID: locID, FolderName: u.FolderName, PartCount: partCount,
	}); err != nil {
		return nil, err
	}

	existing, existingLoc, err := s.q.FindExistingCacheEntryWithLocation(ctx, key, version, scope, repoID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if existing != nil {
		if err := s.q.UpdateCacheEntryLocation(ctx, existing.ID, locID, now); err != nil {
			return nil, err
		}
		if err := s.q.DeleteStorageLocation(ctx, existingLoc.ID); err != nil {
			return nil, err
		}
		if err := s.adapter.DeleteFolder(ctx, existingLoc.FolderName); err != nil {
			return nil, err
		}
	} else {
		if err := s.q.InsertCacheEntry(ctx, dbpkg.CacheEntry{
			ID: ids.UUIDv4(), Key: key, Version: version, Scope: scope, RepoID: repoID,
			UpdatedAt: now, LocationID: locID,
		}); err != nil {
			return nil, err
		}
	}
	if err := s.q.DeleteUpload(ctx, u.ID); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) Download(ctx context.Context, cacheEntryID string) (io.ReadCloser, error) {
	_, loc, err := s.q.FindCacheEntryWithLocation(ctx, cacheEntryID)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, nil
	}
	go func() {
		_ = s.q.UpdateStorageLastDownloaded(context.Background(), loc.ID, time.Now().UnixMilli())
	}()

	if loc.MergedAt != nil {
		r, err := s.adapter.CreateDownloadStream(ctx, loc.FolderName+"/merged")
		if errors.Is(err, ErrObjectNotFound) {
			s.cfg.Logger.Warn("stale cache entry: merged blob missing", "id", cacheEntryID)
			return nil, nil
		}
		return r, err
	}
	if loc.MergeStartedAt != nil {
		return s.streamPartsReader(ctx, loc), nil
	}

	if err := s.ensurePartsExist(ctx, loc); err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			s.cfg.Logger.Warn("stale cache entry: parts missing", "id", cacheEntryID)
			return nil, nil
		}
		return nil, err
	}

	if err := s.q.UpdateStorageMergeStarted(ctx, loc.ID, time.Now().UnixMilli()); err != nil {
		return nil, err
	}

	respR, respW := io.Pipe()
	mergeR, mergeW := io.Pipe()
	done := make(chan struct{})

	s.mergesMu.Lock()
	s.merges[loc.ID] = done
	s.mergesMu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			s.mergesMu.Lock()
			delete(s.merges, loc.ID)
			s.mergesMu.Unlock()
		}()
		bgCtx := context.Background()
		if err := s.adapter.UploadStream(bgCtx, loc.FolderName+"/merged", mergeR); err != nil {
			s.cfg.Logger.Error("merge upload failed", "id", loc.ID, "err", err)
			_ = s.q.ResetStorageMerge(bgCtx, loc.ID)
			return
		}
		if err := s.q.UpdateStorageMerged(bgCtx, loc.ID, time.Now().UnixMilli()); err != nil {
			s.cfg.Logger.Error("mark merged failed", "id", loc.ID, "err", err)
			return
		}
		_ = s.q.UpdateStoragePartsDeleted(bgCtx, loc.ID, time.Now().UnixMilli())
		if err := s.adapter.DeleteFolder(bgCtx, loc.FolderName+"/parts"); err != nil {
			s.cfg.Logger.Error("delete parts after merge failed", "id", loc.ID, "err", err)
		}
	}()

	go func() {
		defer respW.Close()
		defer mergeW.Close()
		mw := io.MultiWriter(respW, mergeW)
		if err := s.streamPartsTo(ctx, loc, mw); err != nil {
			respW.CloseWithError(err)
			mergeW.CloseWithError(err)
		}
	}()

	return respR, nil
}

func (s *Service) ensurePartsExist(ctx context.Context, loc *dbpkg.StorageLocation) error {
	n, err := s.adapter.CountFilesInFolder(ctx, loc.FolderName+"/parts")
	if err != nil {
		return err
	}
	if n < loc.PartCount {
		return ErrObjectNotFound
	}
	return nil
}

func (s *Service) streamPartsTo(ctx context.Context, loc *dbpkg.StorageLocation, w io.Writer) error {
	for i := 0; i < loc.PartCount; i++ {
		r, err := s.adapter.CreateDownloadStream(ctx, fmt.Sprintf("%s/parts/%d", loc.FolderName, i))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, r)
		_ = r.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func (s *Service) streamPartsReader(ctx context.Context, loc *dbpkg.StorageLocation) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if err := s.streamPartsTo(ctx, loc, pw); err != nil {
			pw.CloseWithError(err)
		}
	}()
	return pr
}

type MatchType string

const (
	MatchExactPrimary    MatchType = "exact-primary"
	MatchPrefixedPrimary MatchType = "prefixed-primary"
	MatchExactRestore    MatchType = "exact-restore"
	MatchPrefixedRestore MatchType = "prefixed-restore"
)

type MatchInput struct {
	Keys    []string
	Version string
	Scopes  []string
	RepoID  string
}

type Match struct {
	Entry dbpkg.CacheEntry
	Type  MatchType
}

func (s *Service) MatchCacheEntry(ctx context.Context, in MatchInput) (*Match, error) {
	if len(in.Keys) == 0 {
		return nil, errors.New("at least one key required")
	}
	primary, restore := in.Keys[0], in.Keys[1:]
	for _, scope := range in.Scopes {
		if e, err := s.q.FindExactCacheEntry(ctx, primary, in.Version, scope, in.RepoID); err != nil {
			return nil, err
		} else if e != nil {
			return &Match{Entry: *e, Type: MatchExactPrimary}, nil
		}
		if e, err := s.q.FindPrefixedCacheEntry(ctx, primary, in.Version, scope, in.RepoID); err != nil {
			return nil, err
		} else if e != nil {
			return &Match{Entry: *e, Type: MatchPrefixedPrimary}, nil
		}
		for _, k := range restore {
			if e, err := s.q.FindExactCacheEntry(ctx, k, in.Version, scope, in.RepoID); err != nil {
				return nil, err
			} else if e != nil {
				return &Match{Entry: *e, Type: MatchExactRestore}, nil
			}
			if e, err := s.q.FindPrefixedCacheEntry(ctx, k, in.Version, scope, in.RepoID); err != nil {
				return nil, err
			} else if e != nil {
				return &Match{Entry: *e, Type: MatchPrefixedRestore}, nil
			}
		}
	}
	return nil, nil
}

type ResolvedDownload struct {
	Entry       dbpkg.CacheEntry
	DownloadURL string
}

func (s *Service) GetCacheEntryWithDownloadURL(ctx context.Context, in MatchInput) (*ResolvedDownload, error) {
	m, err := s.MatchCacheEntry(ctx, in)
	if err != nil || m == nil {
		return nil, err
	}
	defaultURL := fmt.Sprintf("%s/download/%s", s.cfg.APIBaseURL, m.Entry.ID)
	if !s.cfg.EnableDirectDownloads {
		return &ResolvedDownload{Entry: m.Entry, DownloadURL: defaultURL}, nil
	}
	loc, err := s.q.GetStorageLocation(ctx, m.Entry.LocationID)
	if err != nil {
		return nil, err
	}
	if loc == nil {
		return nil, errors.New("storage location not found")
	}
	if loc.MergedAt == nil {
		return &ResolvedDownload{Entry: m.Entry, DownloadURL: defaultURL}, nil
	}
	url, ok, err := s.adapter.CreateDownloadURL(ctx, loc.FolderName+"/merged")
	if err != nil {
		return nil, err
	}
	if !ok {
		return &ResolvedDownload{Entry: m.Entry, DownloadURL: defaultURL}, nil
	}
	return &ResolvedDownload{Entry: m.Entry, DownloadURL: url}, nil
}

func (s *Service) Match(ctx context.Context, key, version, scope, repoID string) (*Match, error) {
	return s.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{key}, Version: version, Scopes: []string{scope}, RepoID: repoID,
	})
}

func (s *Service) Q() *dbpkg.DB {
	return s.q.DB()
}

func (s *Service) UsableFreeBytes() (int64, bool, error) {
	r, ok := s.adapter.(DiskUsageReporter)
	if !ok {
		return 0, false, nil
	}
	n, err := r.UsableFreeBytes()
	if err != nil {
		return 0, true, err
	}
	return n, true, nil
}

func (s *Service) OpenMergedSeekable(ctx context.Context, cacheEntryID string) (io.ReadSeekCloser, time.Time, error) {
	_, loc, err := s.q.FindCacheEntryWithLocation(ctx, cacheEntryID)
	if err != nil {
		return nil, time.Time{}, err
	}
	if loc == nil || loc.MergedAt == nil {
		return nil, time.Time{}, nil
	}
	sk, ok := s.adapter.(SeekableAdapter)
	if !ok {
		return nil, time.Time{}, nil
	}
	go func() {
		_ = s.q.UpdateStorageLastDownloaded(context.Background(), loc.ID, time.Now().UnixMilli())
	}()
	r, mt, err := sk.OpenSeekable(ctx, loc.FolderName+"/merged")
	if errors.Is(err, ErrObjectNotFound) {
		s.cfg.Logger.Warn("stale cache entry: merged blob missing", "id", cacheEntryID)
		return nil, time.Time{}, nil
	}
	return r, mt, err
}
