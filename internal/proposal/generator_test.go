package proposal

import "testing"

func TestHasPlaceholderNamedTokens(t *testing.T) {
	cases := []string{
		"Best regards,\n\n[Your Name]",
		"Hello [Name], I can help.",
		"Timeline: [TIMELINE]",
		"I bring [Your Experience] to this project.",
		"[TODO] add portfolio link",
	}
	for _, c := range cases {
		if !HasPlaceholder(c) {
			t.Errorf("expected placeholder detected in %q", c)
		}
	}
}

func TestHasPlaceholderGenericBracket(t *testing.T) {
	cases := []string{
		"Based in [City], I can meet your timezone.",
		"See my work at [Portfolio URL].",
		"Rate: [Hourly Rate]",
	}
	for _, c := range cases {
		if !HasPlaceholder(c) {
			t.Errorf("expected generic bracket detected in %q", c)
		}
	}
}

func TestHasPlaceholderCleanText(t *testing.T) {
	cases := []string{
		"I specialise in Python automation and data extraction.",
		"Best regards,\n\nAlex",
		"Deliverable: a target list of qualified prospects.",
		"",
	}
	for _, c := range cases {
		if HasPlaceholder(c) {
			t.Errorf("clean text flagged as placeholder: %q", c)
		}
	}
}

func TestHasPlaceholderIgnoresMarkdownLinkText(t *testing.T) {
	// Markdown links are common in proposals; the link text must not be read
	// as an unfilled placeholder.
	if HasPlaceholder("Read the case study [here](https://example.com/case).") {
		t.Error("markdown link text should not be flagged as a placeholder")
	}
}
