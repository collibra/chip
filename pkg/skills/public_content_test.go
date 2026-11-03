package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// publicContentRoots are the Guardian Onboarding skill, its shared file and its specification. This repository is
// public, so they must hold nothing but product facts: no internal addresses, people, tickets or real tenant data.
var publicContentRoots = []string{
	"files/collibra/guardian-onboarding",
	"files/collibra/_shared/platform-prerequisites.md",
	"../../docs/guardian-onboarding",
}

// allowedHosts are the only web addresses these files may link to. Anything else, even a Collibra address, fails, so
// the list names no internal host.
var allowedHosts = map[string]bool{
	"productresources.collibra.com": true,
	"github.com":                    true,
	"example.com":                   true,
	"www.python.org":                true,
	"python.org":                    true,
}

var (
	urlPattern     = regexp.MustCompile(`https?://([^/\s)>"'\x60\]]+)(\S*)`)
	emailPattern   = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9.-]+\.[A-Za-z]{2,})`)
	ticketPattern  = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,9}-\d{2,}\b`)
	uuidPattern    = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	personDated    = regexp.MustCompile(`\([A-Z][a-z]+(?: [A-Z][a-z]+)?, \d{4}-\d{2}-\d{2}\)`)
	longNumber     = regexp.MustCompile(`\b\d{12,}\b`)
	sshRemote      = regexp.MustCompile(`git@[A-Za-z0-9.-]+:`)
	allowedTickets = map[string]bool{"SHA-256": true}
)

func TestGuardianOnboardingPublicContent(t *testing.T) {
	for _, root := range publicContentRoots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(body), "\n") {
				for _, problem := range internalDetails(line) {
					t.Errorf("%s:%d: %s: %q", path, i+1, problem, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", root, err)
		}
	}
}

// internalDetails says what in one line looks like something that must not be in a public file.
func internalDetails(line string) []string {
	var problems []string
	for _, m := range urlPattern.FindAllStringSubmatch(line, -1) {
		host, rest := strings.ToLower(m[1]), m[2]
		switch {
		case strings.Contains(host, "<"):
		case host == "github.com" && !strings.HasPrefix(rest, "/collibra/chip"):
			problems = append(problems, "link to a repository other than collibra/chip")
		case !allowedHosts[host]:
			problems = append(problems, "web address not on the allowed list ("+host+")")
		}
	}
	for _, m := range emailPattern.FindAllStringSubmatch(line, -1) {
		if !strings.HasSuffix(strings.ToLower(m[1]), "example.com") {
			problems = append(problems, "email address")
		}
	}
	for _, m := range ticketPattern.FindAllString(line, -1) {
		if !allowedTickets[m] {
			problems = append(problems, "ticket-like id ("+m+")")
		}
	}
	for _, m := range uuidPattern.FindAllString(line, -1) {
		if !strings.HasPrefix(m, "00000000-0000-0000-0000-") {
			problems = append(problems, "id that isn't one of Collibra's fixed ids ("+m+")")
		}
	}
	if personDated.MatchString(line) {
		problems = append(problems, "a name with a date, as in a decision log")
	}
	if longNumber.MatchString(uuidPattern.ReplaceAllString(line, "")) {
		problems = append(problems, "a long number, such as a workspace or account id")
	}
	if sshRemote.MatchString(line) {
		problems = append(problems, "a git remote")
	}
	return problems
}

func TestInternalDetailsCatchesTheKnownKinds(t *testing.T) {
	leaks := []string{
		"see https://wiki.example-corp.net/page",
		"see https://github.com/collibra/some-private-repo",
		"ask jane.doe@acme.io",
		"tracked in ABC-12345",
		"agent 7d3c1a2b-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
		"decided (Jane, 2026-01-02)",
		"workspace 1234567890123456",
		"clone git@code.acme.io:team/repo.git",
	}
	for _, line := range leaks {
		if len(internalDetails(line)) == 0 {
			t.Errorf("not caught: %q", line)
		}
	}
	clean := []string{
		"read https://productresources.collibra.com/docs/collibra/latest/x.htm",
		"open https://<workspace host>/apps/<app name>",
		"relation type 00000000-0000-0000-0000-000000007109",
		"compare the SHA-256 with the reported hash",
		"write to jane@example.com",
	}
	for _, line := range clean {
		if p := internalDetails(line); len(p) > 0 {
			t.Errorf("false alarm on %q: %v", line, p)
		}
	}
}
