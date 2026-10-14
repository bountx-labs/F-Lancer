package bounty

import (
	"testing"
	"time"
)

func baseGateInput() GateInput {
	return GateInput{
		Issue: Issue{
			Title:     "Fix the parser crash",
			Body:      "We pay the bounty in USDC on merge. Small scoped fix.",
			HTMLURL:   "https://github.com/good/repo/issues/1",
			CreatedAt: time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
			Comments:  0,
		},
		Repo: RepoMeta{
			FullName: "good/repo",
			Stars:    500,
			PushedAt: time.Now().UTC().Add(-24 * time.Hour),
		},
		Now: time.Now().UTC(),
	}
}

func TestGatesAllowGoodBounty(t *testing.T) {
	v := EvaluateGates(baseGateInput())
	if !v.Allowed {
		t.Fatalf("expected a healthy bounty to pass, got %s", v)
	}
}

func TestGatesRejectOldBounty(t *testing.T) {
	in := baseGateInput()
	in.Issue.CreatedAt = in.Now.Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	v := EvaluateGates(in)
	if v.Allowed {
		t.Fatal("expected old bounty to be rejected")
	}
}

func TestGatesRejectMissingAge(t *testing.T) {
	in := baseGateInput()
	in.Issue.CreatedAt = ""
	if EvaluateGates(in).Allowed {
		t.Fatal("expected rejection when age cannot be determined")
	}
}

func TestGatesRejectArchivedRepo(t *testing.T) {
	in := baseGateInput()
	in.Repo.Archived = true
	if EvaluateGates(in).Allowed {
		t.Fatal("expected archived repo to be rejected")
	}
}

func TestGatesRejectInactiveRepo(t *testing.T) {
	in := baseGateInput()
	in.Repo.PushedAt = in.Now.Add(-365 * 24 * time.Hour)
	if EvaluateGates(in).Allowed {
		t.Fatal("expected inactive repo to be rejected")
	}
}

func TestGatesRejectUnknownPushDate(t *testing.T) {
	in := baseGateInput()
	in.Repo.PushedAt = time.Time{}
	if EvaluateGates(in).Allowed {
		t.Fatal("expected rejection when push date is unknown")
	}
}

func TestGatesRejectLowStars(t *testing.T) {
	in := baseGateInput()
	in.Repo.Stars = 3
	if EvaluateGates(in).Allowed {
		t.Fatal("expected low-star repo to be rejected")
	}
}

func TestGatesRejectUnknownPaymentRail(t *testing.T) {
	in := baseGateInput()
	in.Issue.Body = "We will pay you in our own coin $FARMTOKEN after review."
	if EvaluateGates(in).Allowed {
		t.Fatal("expected unknown payment rail to be rejected")
	}
}

func TestGatesRejectPriorAttempts(t *testing.T) {
	in := baseGateInput()
	in.PriorAttempts = 5
	if EvaluateGates(in).Allowed {
		t.Fatal("expected heavily engaged issue to be rejected")
	}
}

func TestGatesRejectClaimMarker(t *testing.T) {
	in := baseGateInput()
	in.Issue.Body = "Paid in USDC. /attempt already opened by another dev."
	if EvaluateGates(in).Allowed {
		t.Fatal("expected claim marker to be rejected")
	}
}

func TestGatesRejectScamKeyword(t *testing.T) {
	in := baseGateInput()
	in.Issue.Title = "Bounty Plaza easy money guaranteed reward"
	if EvaluateGates(in).Allowed {
		t.Fatal("expected scam keywords to be rejected")
	}
}

func TestGatesRejectImplausibleReward(t *testing.T) {
	in := baseGateInput()
	in.Issue.Body = "Paid in USDC. Reward $90000 for this tiny fix."
	if EvaluateGates(in).Allowed {
		t.Fatal("expected implausible reward to be rejected")
	}
}

func TestGatesRejectSeedPhraseRequest(t *testing.T) {
	in := baseGateInput()
	in.Issue.Body = "Paid in USDC. Send your seed to receive the reward."
	if EvaluateGates(in).Allowed {
		t.Fatal("expected seed-phrase request to be rejected")
	}
}

func TestGatesCollectAllReasons(t *testing.T) {
	in := baseGateInput()
	in.Repo.Stars = 1
	in.Repo.Archived = true
	in.Issue.Body = "no payment mention here"
	v := EvaluateGates(in)
	if v.Allowed {
		t.Fatal("expected rejection")
	}
	if len(v.Reasons) < 3 {
		t.Errorf("expected multiple reasons, got %d: %v", len(v.Reasons), v.Reasons)
	}
}

func TestGatesUseProvidedNow(t *testing.T) {
	fixed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	in := baseGateInput()
	in.Now = fixed
	in.Issue.CreatedAt = fixed.Add(-1 * time.Hour).Format(time.RFC3339)
	in.Repo.PushedAt = fixed.Add(-48 * time.Hour)
	if !EvaluateGates(in).Allowed {
		t.Fatal("expected pass with fixed clock")
	}
}

func TestVerdictString(t *testing.T) {
	if got := (Verdict{Allowed: true}).String(); got != "pass" {
		t.Errorf("unexpected: %q", got)
	}
	v := Verdict{Allowed: false, Reasons: []string{"one"}}
	if got := v.String(); got == "" {
		t.Error("reject verdict should render reasons")
	}
}
