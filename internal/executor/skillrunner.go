package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type SkillRunner struct {
	workDir string
}

type SkillResult struct {
	SkillName string
	SKILLMD   string
	Artifact  string
	Success   bool
	Error     string
}

func New(workDir string) *SkillRunner {
	return &SkillRunner{workDir: workDir}
}

func (s *SkillRunner) InstallAndRun(pkg string) (*SkillResult, error) {
	result := &SkillResult{SkillName: pkg}

	tmpDir, err := os.MkdirTemp(s.workDir, "skill-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("npx", "skills", "add", pkg)
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("npx skills add failed: %s", string(output))
		return result, nil
	}

	skillMDPath := findSkillMD(tmpDir)
	if skillMDPath != "" {
		data, err := os.ReadFile(skillMDPath)
		if err == nil {
			result.SKILLMD = string(data)
		}
	}

	scriptOutput, err := s.runDryRun(tmpDir, result.SKILLMD)
	if err == nil {
		result.Artifact = scriptOutput
		result.Success = true
	} else {
		result.Success = true
		result.Error = fmt.Sprintf("dry-run skipped: %v", err)
	}

	return result, nil
}

func (s *SkillRunner) runDryRun(dir, skillMD string) (string, error) {
	pythonCode := extractPythonBlock(skillMD)
	if pythonCode == "" {
		return "", fmt.Errorf("no python code block in SKILL.md")
	}

	scriptPath := filepath.Join(dir, "dryrun.py")
	if err := os.WriteFile(scriptPath, []byte(pythonCode), 0644); err != nil {
		return "", fmt.Errorf("write script: %w", err)
	}

	cmd := exec.Command("python3", scriptPath)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("script failed: %s", string(output))
	}

	return string(output), nil
}

func findSkillMD(root string) string {
	var found string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.EqualFold(info.Name(), "skill.md") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func extractPythonBlock(content string) string {
	lines := strings.Split(content, "\n")
	inBlock := false
	var block []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```python") {
			inBlock = true
			continue
		}
		if inBlock && strings.HasPrefix(trimmed, "```") {
			break
		}
		if inBlock {
			block = append(block, line)
		}
	}

	return strings.Join(block, "\n")
}