package skill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseSkillMD parses a SKILL.md file into a Skill.
// Format:
//
//	---
//	name: skill_name
//	description: skill description
//	version: 1.0
//	parameters:
//	  - name: param1
//	    required: true
//	    default: value
//	    description: param description
//	---
//	Template body with {{param1}} placeholders
func ParseSkillMD(content string) (Skill, error) {
	lines := strings.Split(content, "\n")
	var s Skill
	inFrontmatter := false
	frontmatterDone := false
	var bodyLines []string
	var currentParam *Param

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !inFrontmatter && !frontmatterDone {
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				if currentParam != nil {
					s.Parameters = append(s.Parameters, *currentParam)
					currentParam = nil
				}
				inFrontmatter = false
				frontmatterDone = true
				continue
			}
		}

		if inFrontmatter {
			if strings.HasPrefix(trimmed, "- name:") {
				if currentParam != nil {
					s.Parameters = append(s.Parameters, *currentParam)
				}
				currentParam = &Param{
					Name: strings.TrimSpace(strings.TrimPrefix(trimmed, "- name:")),
				}
				continue
			}
			if currentParam != nil {
				if strings.HasPrefix(trimmed, "required:") {
					currentParam.Required = strings.TrimSpace(strings.TrimPrefix(trimmed, "required:")) == "true"
					continue
				}
				if strings.HasPrefix(trimmed, "default:") {
					currentParam.Default = strings.TrimSpace(strings.TrimPrefix(trimmed, "default:"))
					continue
				}
				if strings.HasPrefix(trimmed, "description:") {
					currentParam.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
					continue
				}
			}
			if strings.HasPrefix(trimmed, "name:") && currentParam == nil {
				s.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "name:"))
			} else if strings.HasPrefix(trimmed, "description:") && currentParam == nil {
				s.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			} else if strings.HasPrefix(trimmed, "version:") {
				s.Version = strings.TrimSpace(strings.TrimPrefix(trimmed, "version:"))
			}
			continue
		}

		if frontmatterDone {
			bodyLines = append(bodyLines, line)
		}
	}

	s.Template = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	if s.Name == "" {
		return s, fmt.Errorf("SKILL.md missing name field")
	}
	if s.Template == "" {
		return s, fmt.Errorf("SKILL.md missing template body")
	}
	return s, nil
}

// LoadFromDir scans a directory for SKILL.md files and returns parsed skills.
func LoadFromDir(dir string) ([]Skill, error) {
	var skills []Skill
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading skill dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
			s, err := loadSkillFile(skillFile)
			if err != nil {
				continue // skip invalid skills
			}
			skills = append(skills, s)
		} else if strings.EqualFold(entry.Name(), "SKILL.md") {
			s, err := loadSkillFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}
			skills = append(skills, s)
		}
	}
	return skills, nil
}

func loadSkillFile(path string) (Skill, error) {
	f, err := os.Open(path)
	if err != nil {
		return Skill{}, err
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return Skill{}, err
	}
	return ParseSkillMD(sb.String())
}
