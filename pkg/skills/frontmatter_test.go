package skills

import (
	"slices"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDesc    string
		wantRelated []string
		wantShared  []string
		wantBody    string
	}{
		{
			name:        "no frontmatter",
			input:       "# Title\nbody",
			wantDesc:    "",
			wantRelated: nil,
			wantBody:    "# Title\nbody",
		},
		{
			name: "description only",
			input: `---
description: One-liner.
---

# Body`,
			wantDesc: "One-liner.",
			wantBody: "# Body",
		},
		{
			name: "description and related",
			input: `---
description: With related.
related: collibra/a, collibra/b
---

body`,
			wantDesc:    "With related.",
			wantRelated: []string{"collibra/a", "collibra/b"},
			wantBody:    "body",
		},
		{
			name: "missing closing delimiter is treated as no frontmatter",
			input: `---
description: orphan
# Body without closing fence`,
			wantBody: `---
description: orphan
# Body without closing fence`,
		},
		{
			name: "shared resources",
			input: `---
description: With shared.
shared: a.md, b.md
---

body`,
			wantDesc:   "With shared.",
			wantShared: []string{"a.md", "b.md"},
			wantBody:   "body",
		},
		{
			name: "unrecognized keys ignored",
			input: `---
description: keep this
foo: bar
---

body`,
			wantDesc: "keep this",
			wantBody: "body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body := parseFrontmatter(tt.input)
			if meta.description != tt.wantDesc {
				t.Errorf("description = %q, want %q", meta.description, tt.wantDesc)
			}
			if !slices.Equal(meta.related, tt.wantRelated) {
				t.Errorf("related = %v, want %v", meta.related, tt.wantRelated)
			}
			if !slices.Equal(meta.shared, tt.wantShared) {
				t.Errorf("shared = %v, want %v", meta.shared, tt.wantShared)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
