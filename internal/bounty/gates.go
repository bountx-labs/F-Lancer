package bounty

import (
	"fmt"
	"strings"
	"time"
)

// Gate thresholds. These are deliberately strict: a false negative (skipping a
// real bounty) costs one opportunity, while a false positive (working a scam)
// costs hours of work and produces nothing. Every value here comes from a real
// bounty hunter's field experience, not theory.
const (
	// Real bounties are usually claimed within hours, not days. An issue that
	// has sat open far longer is either already solved, unmaintained, or bait.
	MaxBountyAge = 72 * time.Hour

	// A repository that has not received a push in this long is effectively
	// abandoned, so a merged PR would never be reviewed or paid.
	MaxRepoInactiveAge = 180 * 24 * time.Hour

	// Bounties far above this are almost always fake reward farming. Genuine
	// single-issue bounties in the observed market top out well below this.
	MaxPlausibleReward = 5000.0

	// A repo with no stars at all is almost always a farm or throwaway. Real
	// projects running funded bounties sit just above that, so the bar is kept
	// near zero rather than high: a strict threshold rejects genuine work on
	// small repositories, which is where most real bounties live.
	MinRepoStars = 5

	// Once this many people have already engaged with an issue, the work is
	// likely claimed; joining late rarely pays.
	MaxPriorEngagement = 3
)

// Verdict is the outcome of running every gate against a bounty candidate.
type Verdict struct {
	Allowed bool
	Reasons []string
}

// GateInput is everything the gates need to judge a candidate. It mixes GitHub
// metadata with text pulled from the issue so the currency and staleness checks
// can inspect what a human would read.
//
// The repository detail comes from the dedicated repo endpoint, not the search
// result, because the search payload omits freshness and archive state and can
// report stale star counts.
type GateInput struct {
	Issue         Issue
	Repo          RepoMeta
	PriorAttempts int
	Now           time.Time
}

// KnownGoodRails are payment methods with a real, redeemable value. Anything
// outside this set must be treated as unpayable until proven otherwise.
var KnownGoodRails = []string{
	"usdc",
	"usdt",
	"btc",
	"bitcoin",
	"lightning",
	"sats",
	"eth",
	"stripe",
	"paypal",
	"escrow",
	"algora",
	"opire",
	"bountysource",
	"lightning bounties",
}

// ScamKeywordPatterns are phrases that appear almost exclusively on bait or
// farming issues rather than real funded work.
var ScamKeywordPatterns = []string{
	"bounty plaza",
	"bounty-plaza",
	"easy money",
	"earn passive",
	"guaranteed reward",
	"no experience required",
	"click here to claim",
	"send your wallet",
	"send your seed",
	"send private key",
	"connect your wallet to claim",
	"airdrop",
	"presale",
	"referral bonus",
	"first come first served bounty",
}

// SelfFundingPatterns mark bounties that ask the worker to pay first. These
// are not paid work: the agent would have to spend money to qualify, which is
// the opposite of earning. Any match disqualifies the bounty.
var SelfFundingPatterns = []string{
	"you must fund",
	"funded with at least",
	"post a task",
	"fund a task",
	"fund your own",
	"deposit at least",
	"stake at least",
	"pay to claim",
	"purchase to qualify",
	"buy to participate",
	"requires a deposit",
	"top up to",
}

// EvaluateGates runs every gate and returns a verdict. It fails closed: any
// gate that cannot be verified rejects the candidate. A single failing gate is
// enough to skip the bounty.
func EvaluateGates(in GateInput) Verdict {
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	v := Verdict{Allowed: true}
	reject := func(format string, args ...any) {
		v.Allowed = false
		v.Reasons = append(v.Reasons, fmt.Sprintf(format, args...))
	}

	// Gate 1: age. Old bounties are solved, abandoned, or bait.
	if created, err := parseGitHubTime(in.Issue.CreatedAt); err == nil {
		age := now.Sub(created)
		if age > MaxBountyAge {
			reject("bounty is %s old (max %s); real bounties are claimed within hours", roundDuration(age), MaxBountyAge)
		}
	} else {
		reject("cannot determine bounty age")
	}

	// Gate 2: maintainer/repo activity. A dead repo never merges a PR.
	if in.Repo.Archived {
		reject("repository is archived")
	}
	if in.Repo.PushedAt.IsZero() {
		reject("repository has no recent push data")
	} else if now.Sub(in.Repo.PushedAt) > MaxRepoInactiveAge {
		reject("repository inactive for %s (max %s)", roundDuration(now.Sub(in.Repo.PushedAt)), MaxRepoInactiveAge)
	}
	if in.Repo.Stars < MinRepoStars {
		reject("repository has only %d stars (min %d); too new or throwaway to hold a funded bounty", in.Repo.Stars, MinRepoStars)
	}

	// Gate 3: payment rail. Unknown tokens are not money.
	text := strings.ToLower(in.Issue.Title + " " + in.Issue.Body + " " + labelsText(in.Issue.Labels))
	if !mentionsKnownRail(text) {
		reject("no recognised payout method mentioned (need one of: %s)", strings.Join(KnownGoodRails[:6], ", "))
	}

	// Gate 3b: a paid-work marker must be present. GitHub's "good first issue"
	// and "help wanted" labels mean "please contribute"; they carry no reward,
	// so work matched only through them cannot pay anything. The marker is not
	// required to be a label: a funded program can publish findings as plain
	// issues with an explicit severity and reward and no label at all, and a
	// label-only check silently discarded every one of them.
	if !HasPaidSignal(in.Issue) {
		reject("no paid-work marker found in labels, title, or body")
	}

	// Gate 4: competition. Work already engaged by others is likely taken.
	if in.PriorAttempts >= MaxPriorEngagement {
		reject("%d prior attempts already recorded", in.PriorAttempts)
	}

	// Gate 5: stale status. Claim markers mean the bounty is gone even if the
	// listing still says open.
	if hasClaimMarker(text) {
		reject("issue text contains a claim/attempt marker; likely already taken")
	}

	// Gate 6: scam vocabulary.
	if kw := firstScamKeyword(text); kw != "" {
		reject("scam keyword %q present", kw)
	}

	// Gate 6b: self-funded task. A bounty that requires the worker to pay
	// money to qualify is not income; the worker would be the payer.
	if kw := firstSelfFundingKeyword(text); kw != "" {
		reject("requires the worker to fund the task (%q); not paid work", kw)
	}

	// Gate 7: reward plausibility.
	if amt := BountyHint(in.Issue); amt > MaxPlausibleReward {
		reject("reward %.0f is implausibly high (max %.0f); typical farming bait", amt, MaxPlausibleReward)
	}

	return v
}

// HasPaidLabel reports whether any label marks the issue as paid work.
// Contribution-only labels such as "good first issue" and "help wanted" do not
// count, because they never carry a reward.
func HasPaidLabel(labels []Label) bool {
	for _, l := range labels {
		name := strings.ToLower(strings.TrimSpace(l.Name))
		for _, paid := range PaidBountyLabels {
			if name == strings.ToLower(paid) {
				return true
			}
		}
		// Labels that embed an amount, e.g. "$250" or "bounty: $500".
		if strings.Contains(name, "bounty") || strings.Contains(name, "$") {
			return true
		}
	}
	return false
}

// PaidTextSignals are phrases in an issue title or body that mark it as funded
// work even when no label says so. Funded programs frequently publish findings
// with a stated severity and reward and no label at all, so a label-only check
// discards their entire pipeline. The list holds words that denote a bounty
// program specifically: a bare mention of a payment rail is not one, because
// "we accept USDC" says nothing about whether this issue pays.
var PaidTextSignals = []string{
	"bounty",
	"reward",
	"severity:",
	"severity :",
	"bug bounty",
	"payout",
	"finders fee",
	"finder's fee",
}

// maxTitleBodyScan bounds how much of an issue body is inspected for a paid
// marker. Reward terms appear near the top, so a large body costs time only.
const maxTitleBodyScan = 4000

// HasPaidSignal reports whether an issue carries any marker of funded work,
// checking labels, then the title, then the start of the body. It widens
// HasPaidLabel without dropping the label path, so both labelled and unlabelled
// paid work are recognised.
func HasPaidSignal(issue Issue) bool {
	if HasPaidLabel(issue.Labels) {
		return true
	}
	body := issue.Body
	if len(body) > maxTitleBodyScan {
		body = body[:maxTitleBodyScan]
	}
	text := strings.ToLower(issue.Title + " " + body)
	for _, sig := range PaidTextSignals {
		if strings.Contains(text, sig) {
			return true
		}
	}
	return false
}

// mentionsKnownRail reports whether any trusted payment method appears.
func mentionsKnownRail(lowerText string) bool {
	for _, rail := range KnownGoodRails {
		if strings.Contains(lowerText, rail) {
			return true
		}
	}
	return false
}

// hasClaimMarker detects the conventions bounty hunters use to reserve work.
func hasClaimMarker(lowerText string) bool {
	markers := []string{
		"/attempt", "/claim", "/bounty claim",
		"claimed by", "assigned to", "already solving", "working on this",
		"pr submitted", "fixed in #", "resolved by",
	}
	for _, m := range markers {
		if strings.Contains(lowerText, m) {
			return true
		}
	}
	return false
}

// firstScamKeyword returns the first scam phrase found, or "".
func firstScamKeyword(lowerText string) string {
	for _, kw := range ScamKeywordPatterns {
		if strings.Contains(lowerText, kw) {
			return kw
		}
	}
	return ""
}

// firstSelfFundingKeyword returns the first self-funding phrase found, or "".
func firstSelfFundingKeyword(lowerText string) string {
	for _, kw := range SelfFundingPatterns {
		if strings.Contains(lowerText, kw) {
			return kw
		}
	}
	return ""
}

func labelsText(labels []Label) string {
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		out = append(out, l.Name)
	}
	return strings.Join(out, " ")
}

// parseGitHubTime parses the RFC3339 timestamps GitHub returns.
func parseGitHubTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.TrimSpace(s))
}

// roundDuration keeps log messages readable.
func roundDuration(d time.Duration) time.Duration {
	return d.Round(time.Hour)
}

// String renders a verdict for logging.
func (v Verdict) String() string {
	if v.Allowed {
		return "pass"
	}
	return "reject: " + strings.Join(v.Reasons, "; ")
}
