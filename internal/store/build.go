package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// BuildFile represents a file within a build.
type BuildFile struct {
	Path   string `yaml:"path" json:"path"`
	Size   int64  `yaml:"size" json:"size"`
	SHA256 string `yaml:"sha256" json:"sha256"`
}

// Build represents a numbered, immutable build artifact.
type Build struct {
	Project     string      `yaml:"project" json:"project"`
	BuildNumber int         `yaml:"build_number" json:"build_number"`
	UploadedBy  string      `yaml:"uploaded_by" json:"uploaded_by"`
	UploadedAt  time.Time   `yaml:"uploaded_at" json:"uploaded_at"`
	Files       []BuildFile `yaml:"files" json:"files"`
}

// BuildIndex is the YAML-persisted index of all builds.
type BuildIndex struct {
	Builds []BuildIndexEntry `yaml:"builds"`
}

// BuildIndexEntry is a summary entry in the builds index.
type BuildIndexEntry struct {
	Project     string    `yaml:"project"`
	BuildNumber int       `yaml:"build_number"`
	UploadedBy  string    `yaml:"uploaded_by"`
	UploadedAt  time.Time `yaml:"uploaded_at"`
	FileCount   int       `yaml:"file_count"`
	TotalBytes  int64     `yaml:"total_bytes"`
}

// BuildStore manages build metadata with YAML persistence.
type BuildStore struct {
	mu      sync.Mutex
	dataDir string // root data directory (e.g. /var/lib/hydrarelease)
}

// NewBuildStore creates a new BuildStore rooted at the given data directory.
func NewBuildStore(dataDir string) *BuildStore {
	return &BuildStore{dataDir: dataDir}
}

func (s *BuildStore) indexPath() string {
	return filepath.Join(s.dataDir, "builds.yaml")
}

func (s *BuildStore) buildDir(project string, number int) string {
	return filepath.Join(s.dataDir, "builds", project, fmt.Sprintf("%d", number))
}

func (s *BuildStore) buildPath(project string, number int) string {
	return filepath.Join(s.buildDir(project, number), "build.yaml")
}

func (s *BuildStore) loadIndex() (*BuildIndex, error) {
	data, err := os.ReadFile(s.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &BuildIndex{}, nil
		}
		return nil, fmt.Errorf("reading build index: %w", err)
	}
	var idx BuildIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing build index: %w", err)
	}
	return &idx, nil
}

func (s *BuildStore) saveIndex(idx *BuildIndex) error {
	data, err := yaml.Marshal(idx)
	if err != nil {
		return fmt.Errorf("marshaling build index: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.indexPath()), 0755); err != nil {
		return fmt.Errorf("creating index directory: %w", err)
	}
	return os.WriteFile(s.indexPath(), data, 0644)
}

// nextBuildNumber returns the next build number for a project.
func (s *BuildStore) nextBuildNumber(idx *BuildIndex, project string) int {
	max := 0
	for _, e := range idx.Builds {
		if e.Project == project && e.BuildNumber > max {
			max = e.BuildNumber
		}
	}
	return max + 1
}

// Create registers a new build, assigns a build number, and persists it.
func (s *BuildStore) Create(project, uploadedBy string, files []BuildFile) (*Build, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	number := s.nextBuildNumber(idx, project)
	now := time.Now().UTC()

	build := &Build{
		Project:     project,
		BuildNumber: number,
		UploadedBy:  uploadedBy,
		UploadedAt:  now,
		Files:       files,
	}

	// Persist per-build metadata.
	dir := s.buildDir(project, number)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating build directory: %w", err)
	}

	data, err := yaml.Marshal(build)
	if err != nil {
		return nil, fmt.Errorf("marshaling build: %w", err)
	}
	if err := os.WriteFile(s.buildPath(project, number), data, 0644); err != nil {
		return nil, fmt.Errorf("writing build metadata: %w", err)
	}

	// Update index.
	var totalBytes int64
	for _, f := range files {
		totalBytes += f.Size
	}

	idx.Builds = append(idx.Builds, BuildIndexEntry{
		Project:     project,
		BuildNumber: number,
		UploadedBy:  uploadedBy,
		UploadedAt:  now,
		FileCount:   len(files),
		TotalBytes:  totalBytes,
	})

	if err := s.saveIndex(idx); err != nil {
		return nil, err
	}

	return build, nil
}

// Get retrieves a specific build by project and number.
func (s *BuildStore) Get(project string, number int) (*Build, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.buildPath(project, number))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("build %s/%d not found", project, number)
		}
		return nil, fmt.Errorf("reading build: %w", err)
	}

	var build Build
	if err := yaml.Unmarshal(data, &build); err != nil {
		return nil, fmt.Errorf("parsing build: %w", err)
	}
	return &build, nil
}

// List returns all builds for a project (from the index).
func (s *BuildStore) List(project string) ([]BuildIndexEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	var result []BuildIndexEntry
	for _, e := range idx.Builds {
		if e.Project == project {
			result = append(result, e)
		}
	}
	return result, nil
}

// Stats returns total build count and distinct project count.
func (s *BuildStore) Stats() (buildCount int, projects int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, err := s.loadIndex()
	if err != nil {
		return 0, 0, err
	}

	seen := make(map[string]struct{})
	for _, e := range idx.Builds {
		seen[e.Project] = struct{}{}
	}
	return len(idx.Builds), len(seen), nil
}
