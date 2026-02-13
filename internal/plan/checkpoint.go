package plan

import (
	"fmt"
	"time"
)

// Checkpoint stores a versioned snapshot of a plan.
type Checkpoint struct {
	Version   int       `json:"version"`
	PlanState Plan      `json:"plan_state"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason,omitempty"`
}

// CreateCheckpoint saves the current plan state as a checkpoint.
func (s *Store) CreateCheckpoint(planID string, reason string) (Checkpoint, error) {
	s.mu.Lock()

	p, ok := s.plans[planID]
	if !ok {
		s.mu.Unlock()
		return Checkpoint{}, fmt.Errorf("plan %q not found", planID)
	}

	if s.checkpoints == nil {
		s.checkpoints = make(map[string][]Checkpoint)
	}

	version := len(s.checkpoints[planID]) + 1
	cp := Checkpoint{
		Version:   version,
		PlanState: clonePlan(p),
		CreatedAt: time.Now().UTC(),
		Reason:    reason,
	}
	s.checkpoints[planID] = append(s.checkpoints[planID], cp)
	fn := s.onChange
	s.mu.Unlock()

	if fn != nil {
		fn()
	}
	return cp, nil
}

// Rollback restores a plan to a previous checkpoint version.
func (s *Store) Rollback(planID string, version int) (Plan, error) {
	s.mu.Lock()

	cps, ok := s.checkpoints[planID]
	if !ok || len(cps) == 0 {
		s.mu.Unlock()
		return Plan{}, fmt.Errorf("no checkpoints for plan %q", planID)
	}

	if version < 1 || version > len(cps) {
		s.mu.Unlock()
		return Plan{}, fmt.Errorf("version %d out of range [1, %d]", version, len(cps))
	}

	cp := cps[version-1]
	restored := clonePlan(cp.PlanState)
	restored.UpdatedAt = time.Now().UTC()
	s.plans[planID] = restored
	result := clonePlan(restored)
	fn := s.onChange
	s.mu.Unlock()

	if fn != nil {
		fn()
	}
	return result, nil
}

// ListCheckpoints returns all checkpoints for a plan.
func (s *Store) ListCheckpoints(planID string) []Checkpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cps := s.checkpoints[planID]
	out := make([]Checkpoint, len(cps))
	for i, cp := range cps {
		out[i] = Checkpoint{
			Version:   cp.Version,
			PlanState: clonePlan(cp.PlanState),
			CreatedAt: cp.CreatedAt,
			Reason:    cp.Reason,
		}
	}
	return out
}
