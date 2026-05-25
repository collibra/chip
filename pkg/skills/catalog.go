// Package skills loads and serves skill guides — Markdown documents that
// instruct LLM agents how to compose chip's tools for common Collibra
// workflows. Skills are embedded in the binary at build time and exposed
// via two MCP tools: list_collibra_skills and load_collibra_skill.
package skills

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed all:files
var embeddedFS embed.FS

// Skill is one entry in the catalog. A skill bundles a Markdown body, a
// short description, optional cross-references to related skills, and an
// optional set of reference files that can be loaded on demand.
type Skill struct {
	Name        string
	Description string
	Related     []string
	Body        string
	Resources   []Resource
}

// Resource is a bundled file (e.g. references/column-lineage-workaround.md)
// that a skill can offer for progressive disclosure.
type Resource struct {
	Path    string
	Content string
}

// Catalog is the in-memory index of all skills. Built once at startup.
type Catalog struct {
	byName map[string]*Skill
	order  []string
}

// Load walks the embedded filesystem and returns a populated catalog.
func Load() (*Catalog, error) {
	return loadFromFS(embeddedFS, "files")
}

func loadFromFS(fsys fs.FS, root string) (*Catalog, error) {
	cat := &Catalog{byName: map[string]*Skill{}}
	if err := cat.walk(fsys, root); err != nil {
		return nil, err
	}
	sort.Strings(cat.order)
	return cat, nil
}

func (c *Catalog) walk(fsys fs.FS, root string) error {
	return fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, "/SKILL.md") {
			return nil
		}
		skillDir := path.Dir(p)
		name := strings.TrimPrefix(skillDir, root+"/")
		sharedDir := path.Join(path.Dir(skillDir), "_shared")
		skill, err := loadSkill(fsys, name, skillDir, sharedDir)
		if err != nil {
			return fmt.Errorf("load skill %q: %w", name, err)
		}
		c.byName[name] = skill
		c.order = append(c.order, name)
		return nil
	})
}

func loadSkill(fsys fs.FS, name, dir, sharedDir string) (*Skill, error) {
	raw, err := fs.ReadFile(fsys, path.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}
	meta, body := parseFrontmatter(string(raw))
	resources, err := loadResources(fsys, path.Join(dir, "references"))
	if err != nil {
		return nil, err
	}
	for _, sharedName := range meta.shared {
		content, err := fs.ReadFile(fsys, path.Join(sharedDir, sharedName))
		if err != nil {
			return nil, fmt.Errorf("shared resource %q: %w", sharedName, err)
		}
		resources = append(resources, Resource{
			Path:    "shared/" + sharedName,
			Content: string(content),
		})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Path < resources[j].Path })
	return &Skill{
		Name:        name,
		Description: meta.description,
		Related:     meta.related,
		Body:        body,
		Resources:   resources,
	}, nil
}

func loadResources(fsys fs.FS, dir string) ([]Resource, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Resource
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		content, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, Resource{
			Path:    "references/" + e.Name(),
			Content: string(content),
		})
	}
	return out, nil
}

// Get returns a skill by name, or nil if unknown.
func (c *Catalog) Get(name string) *Skill {
	return c.byName[name]
}

// List returns all skills in deterministic order.
func (c *Catalog) List() []*Skill {
	out := make([]*Skill, 0, len(c.order))
	for _, n := range c.order {
		out = append(out, c.byName[n])
	}
	return out
}

// Search returns skills whose name or description matches the query
// (case-insensitive substring). An empty query returns everything.
func (c *Catalog) Search(query string) []*Skill {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return c.List()
	}
	var out []*Skill
	for _, n := range c.order {
		s := c.byName[n]
		if strings.Contains(strings.ToLower(s.Name), q) ||
			strings.Contains(strings.ToLower(s.Description), q) {
			out = append(out, s)
		}
	}
	return out
}

// Resource looks up a bundled resource on a skill by its relative path
// (e.g. "references/column-lineage-workaround.md").
func (s *Skill) Resource(p string) *Resource {
	for i := range s.Resources {
		if s.Resources[i].Path == p {
			return &s.Resources[i]
		}
	}
	return nil
}

// ResourcePaths returns the list of relative paths for bundled resources.
func (s *Skill) ResourcePaths() []string {
	out := make([]string, len(s.Resources))
	for i, r := range s.Resources {
		out[i] = r.Path
	}
	return out
}
