package freelancer

import "testing"

func TestParseProjectIDFromURL(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"https://www.freelancer.com/projects/web-development/foo/40693857/", 40693857, false},
		{"https://www.freelancer.com/projects/foo/12345678", 12345678, false},
		{"40693857", 40693857, false},
		{"", 0, true},
		{"https://www.freelancer.com/projects/foo/", 0, true},
	}
	for _, tc := range tests {
		got, err := ParseProjectID(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseProjectID(%q) expected error, got %d", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseProjectID(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseProjectID(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestLinkToQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://www.freelancer.com/projects/web-development/build-nextjs-site/", "build nextjs site"},
		{"https://www.freelancer.com/projects/python/scrape-data.html", "scrape data"},
		{"some-slug_here", "some slug here"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := linkToQuery(tc.in); got != tc.want {
			t.Errorf("linkToQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	a := normalizeURL("https://www.freelancer.com/projects/web-development/foo/")
	b := normalizeURL("http://freelancer.com/projects/web-development/foo?ref=x")
	if a != b {
		t.Errorf("expected %q == %q", a, b)
	}
}

func TestParsedBudget(t *testing.T) {
	p := Project{}
	p.Budget.Maximum = 500
	p.Budget.Minimum = 250
	if got := p.ParsedBudget(); got != 500 {
		t.Errorf("expected 500, got %v", got)
	}

	p2 := Project{}
	p2.Budget.Minimum = 100
	if got := p2.ParsedBudget(); got != 100 {
		t.Errorf("expected fallback to minimum 100, got %v", got)
	}
}
