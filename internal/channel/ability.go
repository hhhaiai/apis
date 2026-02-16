package channel

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrChannelNotFound = errors.New("channel not found")
)

// Ability represents a channel's ability to handle a specific model
type Ability struct {
	Group     string `json:"group"`      // User group
	Model     string `json:"model"`      // Model name
	ChannelID int64  `json:"channel_id"` // Channel ID
	Enabled   bool   `json:"enabled"`    // Is this ability active
	Priority  int64  `json:"priority"`   // Higher = more preferred within group
}

// AbilityStore provides in-memory storage for channels and abilities
type AbilityStore struct {
	mu        sync.RWMutex
	channels  map[int64]*Channel
	abilities map[string]*Ability // key: group:model
	byChannel map[int64][]string  // channelID -> []key
	nextID    int64
}

func NewAbilityStore() *AbilityStore {
	return &AbilityStore{
		channels:  make(map[int64]*Channel),
		abilities: make(map[string]*Ability),
		byChannel: make(map[int64][]string),
		nextID:    1,
	}
}

// AddChannel adds a new channel
func (s *AbilityStore) AddChannel(c *Channel) error {
	if c == nil {
		return errors.New("channel is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if c.ID == 0 {
		c.ID = s.nextID
		s.nextID++
	}
	now := time.Now()
	stored := cloneChannel(c)
	stored.CreatedAt = now
	stored.UpdatedAt = now
	s.channels[stored.ID] = stored

	// Generate abilities from channel models
	s.rebuildAbilitiesLocked(stored)
	return nil
}

// UpdateChannel updates an existing channel
func (s *AbilityStore) UpdateChannel(c *Channel) error {
	if c == nil {
		return errors.New("channel is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.channels[c.ID]
	if !ok {
		return ErrChannelNotFound
	}
	stored := cloneChannel(c)
	stored.CreatedAt = existing.CreatedAt
	stored.UpdatedAt = time.Now()
	s.channels[stored.ID] = stored

	// Rebuild abilities if scheduling-related fields changed.
	if existing.Models != stored.Models || existing.Group != stored.Group || existing.Priority != stored.Priority || existing.Status != stored.Status {
		s.rebuildAbilitiesLocked(stored)
	}
	return nil
}

// DeleteChannel removes a channel
func (s *AbilityStore) DeleteChannel(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.channels[id]
	if !ok {
		return ErrChannelNotFound
	}
	delete(s.channels, id)

	// Remove related abilities
	for key, ability := range s.abilities {
		if ability.ChannelID == id {
			delete(s.abilities, key)
		}
	}
	delete(s.byChannel, id)
	return nil
}

// GetChannel returns a channel by ID
func (s *AbilityStore) GetChannel(id int64) (*Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.channels[id]
	if !ok {
		return nil, false
	}
	return cloneChannel(c), true
}

// ListChannels returns all channels
func (s *AbilityStore) ListChannels() []*Channel {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Channel, 0, len(s.channels))
	for _, c := range s.channels {
		result = append(result, cloneChannel(c))
	}
	return result
}

// GetChannelByGroupAndModel finds the best channel for a group and model
func (s *AbilityStore) GetChannelByGroupAndModel(group, model string) (*Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := group + ":" + model

	// Try exact match first
	if ability, ok := s.abilities[key]; ok && ability.Enabled {
		if c, ok := s.channels[ability.ChannelID]; ok && c.IsEnabled() {
			return cloneChannel(c), true
		}
	}

	// Find any enabled channel that can handle this model
	var best *Ability
	for _, ability := range s.abilities {
		if ability.Group == group && ability.Enabled && matchModel(ability.Model, model) {
			if best == nil || ability.Priority > best.Priority {
				best = ability
			}
		}
	}

	if best != nil {
		if c, ok := s.channels[best.ChannelID]; ok && c.IsEnabled() {
			return cloneChannel(c), true
		}
	}

	return nil, false
}

// GetEnabledModels returns all enabled models for a group
func (s *AbilityStore) GetEnabledModels(group string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	modelMap := make(map[string]bool)
	for key, ability := range s.abilities {
		if ability.Group == group && ability.Enabled {
			parts := splitKey(key)
			if len(parts) == 2 {
				modelMap[parts[1]] = true
			}
		}
	}

	result := make([]string, 0, len(modelMap))
	for m := range modelMap {
		result = append(result, m)
	}
	return result
}

// UpdateChannelStatus updates a channel's status
func (s *AbilityStore) UpdateChannelStatus(id int64, status int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, ok := s.channels[id]
	if !ok {
		return ErrChannelNotFound
	}
	c.Status = status
	c.UpdatedAt = time.Now()

	// Rebuild related abilities to ensure status/priority/group/model changes are reflected.
	s.rebuildAbilitiesLocked(c)
	return nil
}

func (s *AbilityStore) rebuildAbilitiesLocked(c *Channel) {
	// Remove old abilities for this channel
	for key, ability := range s.abilities {
		if ability.ChannelID == c.ID {
			delete(s.abilities, key)
		}
	}
	delete(s.byChannel, c.ID)

	// Create new abilities
	models := splitAndTrim(c.Models, ",")
	groups := splitAndTrim(c.Group, ",")

	for _, group := range groups {
		for _, model := range models {
			key := group + ":" + model
			ability := &Ability{
				Group:     group,
				Model:     model,
				ChannelID: c.ID,
				Enabled:   c.IsEnabled(),
				Priority:  c.Priority,
			}
			s.abilities[key] = ability
			s.byChannel[c.ID] = append(s.byChannel[c.ID], key)
		}
	}
}

func cloneChannel(in *Channel) *Channel {
	if in == nil {
		return nil
	}
	out := *in
	if in.BaseURL != nil {
		v := *in.BaseURL
		out.BaseURL = &v
	}
	if in.ModelMapping != nil {
		v := *in.ModelMapping
		out.ModelMapping = &v
	}
	return &out
}

func splitKey(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func matchModel(pattern, model string) bool {
	if pattern == model {
		return true
	}
	if pattern == "*" {
		return true
	}
	// Simple wildcard support
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(model) >= len(prefix) && model[:len(prefix)] == prefix
	}
	return false
}
