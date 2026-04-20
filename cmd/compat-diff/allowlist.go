package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	markerStart = "<!-- compat-harness:deviations:start -->"
	markerEnd   = "<!-- compat-harness:deviations:end -->"
	fenceOpen   = "```yaml"
	fenceClose  = "```"
)

type deviation struct {
	Path     string   `yaml:"path"`
	Kind     string   `yaml:"kind"`
	Reason   string   `yaml:"reason"`
	Fixtures []string `yaml:"fixtures"`
}

type allowlist struct {
	Deviations []deviation `yaml:"deviations"`
}

func loadAllowlist(path string) (allowlist, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return allowlist{}, fmt.Errorf("read allowlist: %w", err)
	}
	block, err := extractYAMLBlock(string(raw))
	if err != nil {
		return allowlist{}, err
	}
	var a allowlist
	if err := yaml.Unmarshal([]byte(block), &a); err != nil {
		return allowlist{}, fmt.Errorf("parse allowlist yaml: %w", err)
	}
	for i, d := range a.Deviations {
		if d.Kind != "ignore" && d.Kind != "allow" {
			return allowlist{}, fmt.Errorf("deviation[%d]: kind = %q, must be ignore|allow", i, d.Kind)
		}
		if d.Path == "" {
			return allowlist{}, fmt.Errorf("deviation[%d]: path is empty", i)
		}
	}
	return a, nil
}

func extractYAMLBlock(doc string) (string, error) {
	startIdx := strings.Index(doc, markerStart)
	if startIdx < 0 {
		return "", errors.New("start marker not found")
	}
	endIdx := strings.Index(doc, markerEnd)
	if endIdx < 0 || endIdx < startIdx {
		return "", errors.New("end marker not found")
	}
	inner := doc[startIdx+len(markerStart) : endIdx]
	fenceStart := strings.Index(inner, fenceOpen)
	if fenceStart < 0 {
		return "", errors.New("yaml fence not found inside markers")
	}
	afterFence := inner[fenceStart+len(fenceOpen):]
	fenceEnd := strings.Index(afterFence, fenceClose)
	if fenceEnd < 0 {
		return "", errors.New("yaml fence not closed")
	}
	return strings.TrimSpace(afterFence[:fenceEnd]), nil
}
