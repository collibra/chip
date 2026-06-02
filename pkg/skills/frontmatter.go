package skills

import "strings"

type frontmatter struct {
	description string
	related     []string
	shared      []string
}

// parseFrontmatter extracts a minimal YAML-like header delimited by `---`
// lines at the top of the document. Recognized keys: description, related,
// shared (the latter two comma-separated). Anything after the closing `---`
// is returned as the body verbatim. If no frontmatter is present, the whole
// input is the body.
func parseFrontmatter(raw string) (frontmatter, string) {
	lines := strings.SplitN(raw, "\n", 2)
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return frontmatter{}, raw
	}
	rest := lines[1]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return frontmatter{}, raw
	}
	header := rest[:end]
	body := strings.TrimPrefix(rest[end+len("\n---"):], "\n")
	body = strings.TrimPrefix(body, "\n")

	meta := frontmatter{}
	for _, line := range strings.Split(header, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "description":
			meta.description = value
		case "related":
			meta.related = splitCSV(value)
		case "shared":
			meta.shared = splitCSV(value)
		}
	}
	return meta, body
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
