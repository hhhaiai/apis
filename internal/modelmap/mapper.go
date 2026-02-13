package modelmap

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

type Mapper interface {
	Resolve(model string) (string, error)
}

type IdentityMapper struct{}

func NewIdentityMapper() *IdentityMapper {
	return &IdentityMapper{}
}

func (m *IdentityMapper) Resolve(model string) (string, error) {
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("model is required")
	}
	return model, nil
}

type StaticMapper struct {
	mapping  map[string]string
	patterns []mapPattern
	strict   bool
	fallback string
}

type mapPattern struct {
	pattern     string
	target      string
	specificity int
}

func NewStaticMapper(mapping map[string]string, strict bool, fallback string) *StaticMapper {
	m := make(map[string]string, len(mapping))
	patterns := make([]mapPattern, 0)
	for k, v := range mapping {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		if strings.Contains(key, "*") {
			patterns = append(patterns, mapPattern{
				pattern:     key,
				target:      val,
				specificity: len(strings.ReplaceAll(key, "*", "")),
			})
		} else {
			m[key] = val
		}
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].specificity == patterns[j].specificity {
			return patterns[i].pattern < patterns[j].pattern
		}
		return patterns[i].specificity > patterns[j].specificity
	})
	return &StaticMapper{
		mapping:  m,
		patterns: patterns,
		strict:   strict,
		fallback: strings.TrimSpace(fallback),
	}
}

func (m *StaticMapper) Resolve(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("model is required")
	}
	if target, ok := m.mapping[model]; ok {
		return target, nil
	}
	for _, p := range m.patterns {
		matched, err := path.Match(p.pattern, model)
		if err != nil {
			continue
		}
		if matched {
			return p.target, nil
		}
	}
	if m.fallback != "" {
		return m.fallback, nil
	}
	if m.strict {
		return "", fmt.Errorf("model %q is not mapped", model)
	}
	return model, nil
}

func NewFromEnv() (Mapper, error) {
	rawMap := strings.TrimSpace(os.Getenv("MODEL_MAP_JSON"))
	if rawMap == "" {
		rawMap = "{}"
	}
	mapping, err := parseModelMapJSON(rawMap)
	if err != nil {
		return nil, err
	}

	strict := strings.EqualFold(strings.TrimSpace(os.Getenv("MODEL_MAP_STRICT")), "true")
	fallback := strings.TrimSpace(os.Getenv("MODEL_MAP_FALLBACK"))

	if len(mapping) == 0 && !strict && fallback == "" {
		return NewIdentityMapper(), nil
	}
	return NewStaticMapper(mapping, strict, fallback), nil
}

func parseModelMapJSON(raw string) (map[string]string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("invalid MODEL_MAP_JSON: %w", err)
	}
	return m, nil
}
