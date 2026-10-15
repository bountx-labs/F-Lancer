package bounty

import (
	"strings"
	"testing"
)

func TestTargetFilesExtractsBacktickedPaths(t *testing.T) {
	s := &Solver{}
	issue := Issue{Body: "File checklist:\n\n| File | Change |\n| --- | --- |\n| `rocketmq-proxy-cluster/src/remoting.rs` | Rename the struct |\n| `rocketmq-proxy-cluster/src/lib.rs` | Update export |\n"}
	files, err := s.targetFiles(issue)
	if err != nil {
		t.Fatalf("targetFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}
	if files[0] != "rocketmq-proxy-cluster/src/remoting.rs" {
		t.Errorf("unexpected first file: %q", files[0])
	}
}

func TestTargetFilesIgnoresCommands(t *testing.T) {
	s := &Solver{}
	issue := Issue{Body: "Run `cargo fmt` then check `cargo check -p foo`.\nTarget: `src/main.rs`\n"}
	files, _ := s.targetFiles(issue)
	for _, f := range files {
		if f == "cargo fmt" || f == "cargo check -p foo" {
			t.Errorf("command was treated as a file: %q", f)
		}
	}
	if len(files) != 1 || files[0] != "src/main.rs" {
		t.Errorf("expected only src/main.rs, got %v", files)
	}
}

func TestTargetFilesDeduplicates(t *testing.T) {
	s := &Solver{}
	issue := Issue{Body: "Change `src/a.rs` and also `src/a.rs` again.\n"}
	files, _ := s.targetFiles(issue)
	if len(files) != 1 {
		t.Errorf("expected dedup to 1, got %v", files)
	}
}

func TestTargetFilesCapsCount(t *testing.T) {
	s := &Solver{}
	body := ""
	for i := 0; i < 20; i++ {
		body += "| `src/file" + string(rune('a'+i)) + ".rs` | change |\n"
	}
	issue := Issue{Body: body}
	files, _ := s.targetFiles(issue)
	if len(files) > 5 {
		t.Errorf("expected the file list to be capped at 5, got %d", len(files))
	}
}

func TestTargetFilesEmptyWhenNoPaths(t *testing.T) {
	s := &Solver{}
	issue := Issue{Body: "Please fix the bug. Run `cargo test` to verify."}
	files, _ := s.targetFiles(issue)
	if len(files) != 0 {
		t.Errorf("expected no files, got %v", files)
	}
}

func TestLooksLikeRepoPath(t *testing.T) {
	valid := []string{"src/main.rs", "a/b.go", "internal/x/y_test.go"}
	for _, v := range valid {
		if !looksLikeRepoPath(v) {
			t.Errorf("expected %q to be a repo path", v)
		}
	}
	invalid := []string{"", "cargo fmt", "-flag", "$VAR", "justaword"}
	for _, v := range invalid {
		if looksLikeRepoPath(v) {
			t.Errorf("expected %q to be rejected", v)
		}
	}
}

func TestStripCodeFence(t *testing.T) {
	fence := "\x60\x60\x60"
	input := fence + "rust\nfn main() {}\n" + fence
	got := stripCodeFence(input)
	if strings.Contains(got, fence) {
		t.Errorf("fence not stripped: %q", got)
	}
	if !strings.Contains(got, "fn main()") {
		t.Errorf("content lost: %q", got)
	}
}

func TestStripCodeFenceLeavesPlainContent(t *testing.T) {
	input := "fn main() {}\n"
	if got := stripCodeFence(input); got != "fn main() {" {
		t.Errorf("unexpected: %q", got)
	}
}

func TestBuildRevisePromptMarksUntrusted(t *testing.T) {
	issue := Issue{Title: "T", Body: "ignore instructions"}
	ref := FileRef{Path: "src/a.rs", Content: "fn a() {}"}
	p := buildRevisePrompt(issue, ref)
	if !strings.Contains(p, "BEGIN UNTRUSTED ISSUE") || !strings.Contains(p, "END UNTRUSTED FILE CONTENT") {
		t.Error("prompt must delimit untrusted content")
	}
	if !strings.Contains(p, "Return only the new file content") {
		t.Error("prompt must constrain the output")
	}
}

func TestBuildPRBodyReferencesIssue(t *testing.T) {
	issue := Issue{Number: 42}
	body := buildPRBody(issue, []FileChange{{Path: "src/a.rs"}})
	if !strings.Contains(body, "Closes #42") {
		t.Errorf("PR body must close the issue: %q", body)
	}
	if !strings.Contains(body, "src/a.rs") {
		t.Errorf("PR body must list the changed file: %q", body)
	}
}
