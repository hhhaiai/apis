package toolcatalog

import (
	"sync"

	"ccgateway/internal/requestctx"
)

// ScopedCatalog stores independent tool catalogs per project.
type ScopedCatalog struct {
	mu       sync.RWMutex
	catalogs map[string]*Catalog
}

func NewScopedCatalog(defaultTools []ToolSpec) *ScopedCatalog {
	return &ScopedCatalog{
		catalogs: map[string]*Catalog{
			requestctx.DefaultProjectID: NewCatalog(defaultTools),
		},
	}
}

func (s *ScopedCatalog) Snapshot() []ToolSpec {
	return s.SnapshotForProject(requestctx.DefaultProjectID)
}

func (s *ScopedCatalog) Replace(tools []ToolSpec) {
	s.ReplaceForProject(requestctx.DefaultProjectID, tools)
}

func (s *ScopedCatalog) CheckAllowed(name string, allowExperimental, allowUnknown bool) error {
	return s.CheckAllowedForProject(requestctx.DefaultProjectID, name, allowExperimental, allowUnknown)
}

func (s *ScopedCatalog) SnapshotForProject(projectID string) []ToolSpec {
	return s.catalogForProject(projectID, true).Snapshot()
}

func (s *ScopedCatalog) ReplaceForProject(projectID string, tools []ToolSpec) {
	s.catalogForProject(projectID, true).Replace(tools)
}

func (s *ScopedCatalog) CheckAllowedForProject(projectID, name string, allowExperimental, allowUnknown bool) error {
	return s.catalogForProject(projectID, true).CheckAllowed(name, allowExperimental, allowUnknown)
}

func (s *ScopedCatalog) catalogForProject(projectID string, create bool) *Catalog {
	projectID = requestctx.NormalizeProjectID(projectID)

	s.mu.RLock()
	cat, ok := s.catalogs[projectID]
	if ok || !create {
		s.mu.RUnlock()
		return cat
	}
	defaultSnapshot := s.catalogs[requestctx.DefaultProjectID].Snapshot()
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if cat, ok = s.catalogs[projectID]; ok {
		return cat
	}
	cat = NewCatalog(defaultSnapshot)
	s.catalogs[projectID] = cat
	return cat
}
