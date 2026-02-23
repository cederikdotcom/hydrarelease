package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Release represents a build promoted to an environment.
type Release struct {
	Project             string    `yaml:"project" json:"project"`
	Environment         string    `yaml:"environment" json:"environment"`
	BuildNumber         int       `yaml:"build_number" json:"build_number"`
	Version             string    `yaml:"version" json:"version"`
	ReleasedBy          string    `yaml:"released_by" json:"released_by"`
	ReleasedAt          time.Time `yaml:"released_at" json:"released_at"`
	ReleaseNotes        string    `yaml:"release_notes,omitempty" json:"release_notes,omitempty"`
	PreviousBuildNumber int       `yaml:"previous_build_number,omitempty" json:"previous_build_number,omitempty"`
}

// ReleaseIndex is the YAML-persisted index of all release promotions.
type ReleaseIndex struct {
	Releases []ReleaseIndexEntry `yaml:"releases"`
}

// ReleaseIndexEntry is a summary entry in the releases index.
type ReleaseIndexEntry struct {
	Project     string    `yaml:"project"`
	Environment string    `yaml:"environment"`
	BuildNumber int       `yaml:"build_number"`
	Version     string    `yaml:"version"`
	ReleasedBy  string    `yaml:"released_by"`
	ReleasedAt  time.Time `yaml:"released_at"`
}

// ReleaseStore manages release metadata with YAML persistence.
type ReleaseStore struct {
	mu      sync.Mutex
	dataDir string // root data directory
}

// NewReleaseStore creates a new ReleaseStore.
func NewReleaseStore(dataDir string) *ReleaseStore {
	return &ReleaseStore{dataDir: dataDir}
}

func (s *ReleaseStore) indexPath() string {
	return filepath.Join(s.dataDir, "releases.yaml")
}

func (s *ReleaseStore) releasePath(project, env string) string {
	return filepath.Join(s.dataDir, "releases", project, env, "release.yaml")
}

func (s *ReleaseStore) loadIndex() (*ReleaseIndex, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &ReleaseIndex{}, nil
		}
		return nil, fmt.Errorf("reading release index: %w", err)
	}
	var idx ReleaseIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing release index: %w", err)
	}
	return &idx, nil
}

func (s *ReleaseStore) saveIndex(idx *ReleaseIndex) error {
	data, err := yaml.Marshal(idx)
	if err != nil {
		return fmt.Errorf("marshaling release index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.indexPath()), 0755); err != nil {
		return fmt.Errorf("creating index directory: %w", err)
	}
	return atomicWriteFile(s.indexPath(), data, 0644)
}

func (s *ReleaseStore) loadRelease(project, env string) (*Release, error) {
	data, err := os.ReadFile(s.releasePath(project, env))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading release: %w", err)
	}
	var rel Release
	if err := yaml.Unmarshal(data, &rel); err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}
	return &rel, nil
}

func (s *ReleaseStore) saveRelease(rel *Release) error {
	path := s.releasePath(rel.Project, rel.Environment)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating release directory: %w", err)
	}
	data, err := yaml.Marshal(rel)
	if err != nil {
		return fmt.Errorf("marshaling release: %w", err)
	}
	return atomicWriteFile(path, data, 0644)
}

// PromoteRequest contains the parameters for promoting a build.
type PromoteRequest struct {
	Project      string
	Environment  string
	BuildNumber  int
	Version      string
	ReleasedBy   string
	ReleaseNotes string
}

// Promote promotes a build to an environment, persists state, and writes latest.json.
func (s *ReleaseStore) Promote(req PromoteRequest) (*Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	// Load current release to get previous build number.
	current, err := s.loadRelease(req.Project, req.Environment)
	if err != nil {
		return nil, err
	}

	var previousBuild int
	if current != nil {
		previousBuild = current.BuildNumber
	}

	now := time.Now().UTC()
	rel := &Release{
		Project:             req.Project,
		Environment:         req.Environment,
		BuildNumber:         req.BuildNumber,
		Version:             req.Version,
		ReleasedBy:          req.ReleasedBy,
		ReleasedAt:          now,
		ReleaseNotes:        req.ReleaseNotes,
		PreviousBuildNumber: previousBuild,
	}

	// Save per-env release state.
	if err := s.saveRelease(rel); err != nil {
		return nil, err
	}

	// Append to index.
	idx.Releases = append(idx.Releases, ReleaseIndexEntry{
		Project:     req.Project,
		Environment: req.Environment,
		BuildNumber: req.BuildNumber,
		Version:     req.Version,
		ReleasedBy:  req.ReleasedBy,
		ReleasedAt:  now,
	})
	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}

	return rel, nil
}

// Rollback rolls back to the previous build in an environment.
func (s *ReleaseStore) Rollback(project, env, rolledBackBy string) (*Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.loadRelease(project, env)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("no release found for %s/%s", project, env)
	}
	if current.PreviousBuildNumber == 0 {
		return nil, fmt.Errorf("no previous build to roll back to for %s/%s", project, env)
	}

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	// Find the previous release entry to get its version.
	var prevVersion string
	for i := len(idx.Releases) - 1; i >= 0; i-- {
		e := idx.Releases[i]
		if e.Project == project && e.Environment == env && e.BuildNumber == current.PreviousBuildNumber {
			prevVersion = e.Version
			break
		}
	}
	if prevVersion == "" {
		prevVersion = current.Version // fallback
	}

	now := time.Now().UTC()
	rel := &Release{
		Project:             project,
		Environment:         env,
		BuildNumber:         current.PreviousBuildNumber,
		Version:             prevVersion,
		ReleasedBy:          rolledBackBy,
		ReleasedAt:          now,
		ReleaseNotes:        fmt.Sprintf("Rollback from build %d", current.BuildNumber),
		PreviousBuildNumber: current.BuildNumber,
	}

	if err := s.saveRelease(rel); err != nil {
		return nil, err
	}

	idx.Releases = append(idx.Releases, ReleaseIndexEntry{
		Project:     project,
		Environment: env,
		BuildNumber: rel.BuildNumber,
		Version:     prevVersion,
		ReleasedBy:  rolledBackBy,
		ReleasedAt:  now,
	})
	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}

	return rel, nil
}

// Get returns the current release for a project/environment.
func (s *ReleaseStore) Get(project, env string) (*Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rel, err := s.loadRelease(project, env)
	if err != nil {
		return nil, err
	}
	if rel == nil {
		return nil, fmt.Errorf("no release found for %s/%s", project, env)
	}
	return rel, nil
}

// List returns all release history for a project (from the index).
func (s *ReleaseStore) List(project string) ([]ReleaseIndexEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	var result []ReleaseIndexEntry
	for _, e := range idx.Releases {
		if e.Project == project {
			result = append(result, e)
		}
	}
	return result, nil
}

// ListCurrentReleases returns the current release for every project/environment
// by scanning the releases directory. Used to pre-populate the latest map on startup.
func (s *ReleaseStore) ListCurrentReleases() ([]Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	releasesDir := filepath.Join(s.dataDir, "releases")
	projects, err := os.ReadDir(releasesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading releases directory: %w", err)
	}

	var result []Release
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		envs, err := os.ReadDir(filepath.Join(releasesDir, p.Name()))
		if err != nil {
			continue
		}
		for _, e := range envs {
			if !e.IsDir() {
				continue
			}
			rel, err := s.loadRelease(p.Name(), e.Name())
			if err != nil || rel == nil {
				continue
			}
			result = append(result, *rel)
		}
	}
	return result, nil
}

// Stats returns the total number of release promotions.
func (s *ReleaseStore) Stats() (releaseCount int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return 0, err
	}
	return len(idx.Releases), nil
}
