package bounty

import (
	"strings"
	"testing"
)

func TestClaimCommentWithoutAddress(t *testing.T) {
	got := claimComment("")
	if !strings.Contains(got, "I would like to work on this") {
		t.Errorf("claim must state intent: %q", got)
	}
	if strings.Contains(got, "Payout") {
		t.Errorf("no address means no payout line: %q", got)
	}
}

func TestClaimCommentIncludesAddress(t *testing.T) {
	got := claimComment("  0xABC123  ")
	if !strings.Contains(got, "0xABC123") {
		t.Errorf("claim must carry the payout address: %q", got)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("address should be trimmed into the claim: %q", got)
	}
}

func TestClaimCommentIgnoresBlankAddress(t *testing.T) {
	if strings.Contains(claimComment("   "), "Payout") {
		t.Error("a blank address must not produce a payout line")
	}
}
