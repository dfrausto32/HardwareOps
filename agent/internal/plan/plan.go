package plan

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type Plan struct {
	Version string  `yaml:"version"`
	Steps   []Step  `yaml:"steps"`
	Health  *Health `yaml:"health"`
}

type Step struct {
	ID         string         `yaml:"id"`
	Type       string         `yaml:"type"`
	TimeoutSec *int           `yaml:"timeoutSec"`
	OnFail     string         `yaml:"onFail"`
	Params     map[string]any `yaml:"params"`
}

type Health struct {
	Type         string `yaml:"type"`
	URL          string `yaml:"url"`
	ExpectStatus *int   `yaml:"expectStatus"`
	TimeoutSec   *int   `yaml:"timeoutSec"`
	IntervalSec  *int   `yaml:"intervalSec"`
}

var supportedStepTypes = map[string]struct{}{
	"file.copy":       {},
	"file.render":     {},
	"symlink.switch":  {},
	"probe.http":      {},
	"script.preApply": {},
}

var requiredParams = map[string][]string{
	"file.copy":       {"src", "dest"},
	"file.render":     {"template", "dest"},
	"symlink.switch":  {"link", "target"},
	"probe.http":      {"url"},
	"script.preApply": {"command"},
}

func Parse(data []byte) (*Plan, error) {
	var p Plan
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	return &p, nil
}

func Validate(p Plan) error {
	if strings.TrimSpace(p.Version) == "" {
		return fmt.Errorf("missing plan version")
	}

	if p.Health != nil {
		if strings.TrimSpace(p.Health.Type) == "" {
			return fmt.Errorf("health: missing type")
		}
		if p.Health.Type != "probe.http" {
			return fmt.Errorf("health: unsupported type %q", p.Health.Type)
		}
		if strings.TrimSpace(p.Health.URL) == "" {
			return fmt.Errorf("health: missing url")
		}
		if p.Health.ExpectStatus != nil && *p.Health.ExpectStatus <= 0 {
			return fmt.Errorf("health: expectStatus must be > 0")
		}
		if p.Health.TimeoutSec != nil && *p.Health.TimeoutSec < 0 {
			return fmt.Errorf("health: timeoutSec must be >= 0")
		}
		if p.Health.IntervalSec != nil && *p.Health.IntervalSec <= 0 {
			return fmt.Errorf("health: intervalSec must be > 0")
		}
	}

	seen := make(map[string]struct{}, len(p.Steps))
	for i, step := range p.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("steps[%d]: missing id", i)
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("steps[%d]: duplicate id %q", i, step.ID)
		}
		seen[step.ID] = struct{}{}

		if strings.TrimSpace(step.Type) == "" {
			return fmt.Errorf("steps[%d]: missing type", i)
		}
		if _, ok := supportedStepTypes[step.Type]; !ok {
			return fmt.Errorf("steps[%d]: unsupported type %q", i, step.Type)
		}

		if step.TimeoutSec != nil && *step.TimeoutSec < 0 {
			return fmt.Errorf("steps[%d]: timeoutSec must be >= 0", i)
		}

		if strings.TrimSpace(step.OnFail) == "" {
			return fmt.Errorf("steps[%d]: missing onFail", i)
		}
		if step.OnFail != "abort" && step.OnFail != "rollback" {
			return fmt.Errorf("steps[%d]: invalid onFail %q", i, step.OnFail)
		}

		if step.Params == nil {
			return fmt.Errorf("steps[%d]: missing params", i)
		}
		for _, key := range requiredParams[step.Type] {
			if _, ok := step.Params[key]; !ok {
				return fmt.Errorf("steps[%d]: missing params.%s", i, key)
			}
		}
	}

	return nil
}
