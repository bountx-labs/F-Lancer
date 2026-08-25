package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Output and execution bounds for skill packages. Skills run third-party
// code, so every subprocess is time-limited and its output is capped to
// keep a misbehaving package from stalling or exhausting the runner.
const (
	skillInstallTimeout = 60 * time.Second
	skillRunTimeout     = 30 * time.Second
	maxSkillOutputBytes = 64 << 10  // 64 KiB
	maxSkillMDBytes     = 256 << 10 // 256 KiB
)

// packageNamePattern accepts npm-style names: plain (pkg), scoped
// (@scope/pkg), and optionally version-pinned (pkg@1.2.3). It rejects
// whitespace and shell metacharacters so a malformed registry entry cannot
// smuggle extra arguments into exec.Command.
var packageNamePattern = regexp.MustCompile(`^@?[a-zA-Z0-9][a-zA-Z0-9._-]*(/[a-zA-Z0-9][a-zA-Z0-9._-]*)?(@[0-9]+(\.[0-9]+){0,2})?$`)

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

	if !packageNamePattern.MatchString(pkg) {
		result.Success = false
		result.Error = fmt.Sprintf("package name %q rejected: not a valid npm-style name", pkg)
		return result, nil
	}

	tmpDir, err := os.MkdirTemp(s.workDir, "skill-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	npxPath, err := exec.LookPath("npx")
	if err != nil {
		result.Success = false
		result.Error = "npx not found on PATH; skill skipped"
		return result, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), skillInstallTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, npxPath, "skills", "add", pkg, "--yes")
	cmd.Dir = tmpDir
	output, err := boundedOutput(cmd)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("npx skills add failed: %s", output)
		return result, nil
	}

	skillMDPath := findSkillMD(tmpDir)
	if skillMDPath != "" {
		data, err := os.ReadFile(skillMDPath)
		if err == nil {
			if len(data) > maxSkillMDBytes {
				data = data[:maxSkillMDBytes]
			}
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

	ctx, cancel := context.WithTimeout(context.Background(), skillRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "python3", scriptPath)
	cmd.Dir = dir
	output, err := boundedOutput(cmd)
	if err != nil {
		return output, fmt.Errorf("script failed: %s", output)
	}

	return output, nil
}

// boundedOutput runs cmd and captures combined stdout/stderr capped at
// maxSkillOutputBytes so a noisy or malicious skill cannot exhaust memory.
func boundedOutput(cmd *exec.Cmd) (string, error) {
	var buf limitedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// limitedBuffer drops bytes beyond maxSkillOutputBytes while reporting that
// it accepted them, so writers never block on a full pipe.
type limitedBuffer struct {
	bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	room := maxSkillOutputBytes - b.Len()
	if room <= 0 {
		return len(p), nil
	}
	if len(p) > room {
		p = p[:room]
	}
	return b.Buffer.Write(p)
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
