package executor

import "testing"

func TestPackageNamePatternAcceptsValidNames(t *testing.T) {
	valid := []string{
		"just-scrape",
		"scrapegraphai/just-scrape",
		"pkg@1.2.3",
		"@scope/pkg",
		"@scope/pkg@2.0",
		"marketingskills/seo-audit",
	}
	for _, name := range valid {
		if !packageNamePattern.MatchString(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
}

func TestPackageNamePatternRejectsInjection(t *testing.T) {
	invalid := []string{
		"",
		"pkg; rm -rf /",
		"pkg && evil",
		"pkg --flag",
		"../escape",
		"pkg name",
		"$(whoami)",
	}
	for _, name := range invalid {
		if packageNamePattern.MatchString(name) {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestLimitedBufferCapsOutput(t *testing.T) {
	var buf limitedBuffer
	big := make([]byte, maxSkillOutputBytes+1000)
	for i := range big {
		big[i] = 'a'
	}
	n, err := buf.Write(big)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(big) {
		t.Errorf("expected write to report all bytes accepted, got %d", n)
	}
	if buf.Len() > maxSkillOutputBytes {
		t.Errorf("buffer exceeded cap: %d", buf.Len())
	}
}

func TestLimitedBufferAcceptsSmallWrite(t *testing.T) {
	var buf limitedBuffer
	buf.Write([]byte("hello"))
	if buf.String() != "hello" {
		t.Errorf("unexpected buffer content: %q", buf.String())
	}
}

func TestExtractPythonBlock(t *testing.T) {
	content := "intro\n```python\nprint('hi')\nx = 1\n```\ntrailing"
	got := extractPythonBlock(content)
	want := "print('hi')\nx = 1"
	if got != want {
		t.Errorf("extractPythonBlock = %q, want %q", got, want)
	}
}

func TestExtractPythonBlockMissing(t *testing.T) {
	if got := extractPythonBlock("no code here"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractPythonBlockStopsAtFence(t *testing.T) {
	content := "```python\nline1\n```\nline2"
	if got := extractPythonBlock(content); got != "line1" {
		t.Errorf("expected only line1, got %q", got)
	}
}
