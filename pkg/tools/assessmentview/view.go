// Package assessmentview projects the Assessments API DTOs onto the shape the
// MCP tools expose.
//
// The API carries an answer value as an untyped JSON value, because its shape
// depends on the answer type (string, number, bool, or a list of options). A
// tool schema must declare a concrete type for every field: an untyped Go field
// makes the reflector emit a subschema with no type at all, which strict MCP
// clients reject — and one rejected tool can fail a client's whole tool import.
// So a scalar answer is projected onto Value, and a choice answer onto Items,
// which also matches the shape edit_assessment accepts as input.
package assessmentview

import (
	"encoding/json"
	"strconv"

	"github.com/collibra/chip/pkg/clients"
)

// Assessment is a conducted assessment as the tools expose it.
type Assessment struct {
	ID                  string              `json:"id" jsonschema:"UUID of the assessment"`
	Name                string              `json:"name,omitempty" jsonschema:"name of the assessment"`
	Status              string              `json:"status,omitempty" jsonschema:"DRAFT, SUBMITTED, or OBSOLETE"`
	Template            *Template           `json:"template,omitempty" jsonschema:"the template the assessment was conducted from"`
	Content             []QuestionAndAnswer `json:"content,omitempty" jsonschema:"the assessment's questions with their current answers"`
	Asset               *Ref                `json:"asset,omitempty" jsonschema:"the asset the assessment was conducted on"`
	AssessmentReview    *Ref                `json:"assessmentReview,omitempty" jsonschema:"the Assessment Review catalog asset linked to the assessment"`
	Assignees           []Assignee          `json:"assignees,omitempty" jsonschema:"the users and groups assigned to the assessment"`
	Owner               *Ref                `json:"owner,omitempty" jsonschema:"the user who owns the assessment"`
	IsVisibleToEveryone *bool               `json:"isVisibleToEveryone,omitempty" jsonschema:"true if every user can see the assessment"`
	CreatedOn           string              `json:"createdOn,omitempty" jsonschema:"ISO-8601 timestamp of the creation"`
	LastModifiedOn      string              `json:"lastModifiedOn,omitempty" jsonschema:"ISO-8601 timestamp of the last change"`
	SubmittedOn         string              `json:"submittedOn,omitempty" jsonschema:"ISO-8601 timestamp of the submission, if the assessment was submitted"`
}

// Ref is the {id, name} reference shape.
type Ref struct {
	ID   string `json:"id" jsonschema:"UUID of the referenced object"`
	Name string `json:"name,omitempty" jsonschema:"name of the referenced object"`
}

// Template is the template metadata. It does not carry the template's
// questions — those come from the assessment's content.
type Template struct {
	ID        string `json:"id" jsonschema:"UUID of the template"`
	Name      string `json:"name,omitempty" jsonschema:"name of the template"`
	Version   string `json:"version,omitempty" jsonschema:"version of the template"`
	Status    string `json:"status,omitempty" jsonschema:"DRAFT, PUBLISHED, or OBSOLETE"`
	AssetType *Ref   `json:"assetType,omitempty" jsonschema:"the asset type the template applies to"`
}

// Assignee is a user or group assigned to the assessment.
type Assignee struct {
	ID   string `json:"id" jsonschema:"UUID of the user or group"`
	Type string `json:"type" jsonschema:"USER or GROUP"`
}

// QuestionAndAnswer is one question with its current answer.
type QuestionAndAnswer struct {
	ID          string  `json:"id" jsonschema:"question id — pass this to edit_assessment to set the answer"`
	Name        string  `json:"name,omitempty" jsonschema:"the question text"`
	Description string  `json:"description,omitempty" jsonschema:"more detail about the question"`
	Answer      *Answer `json:"answer,omitempty" jsonschema:"the current answer, absent while the question is unanswered"`
	Comments    string  `json:"comments,omitempty" jsonschema:"free-text comment attached to the answer"`
}

// Answer is a typed answer. A scalar answer uses Value; a choice (ITEMS)
// answer uses Items.
type Answer struct {
	Type  string `json:"type" jsonschema:"the answer type: TEXT, HTML, EXPRESSION, NUMBER, BOOLEAN, DATE, or ITEMS"`
	Value string `json:"value,omitempty" jsonschema:"the answer value as a string, for every type except ITEMS. A NUMBER reads as digits and a BOOLEAN as 'true'/'false'. Pass it back to edit_assessment as 'value'"`
	Items []Item `json:"items,omitempty" jsonschema:"the selected options, for an ITEMS answer. Pass them back to edit_assessment as 'items'"`
}

// Item is one selected option of a choice answer.
type Item struct {
	ID    string `json:"id" jsonschema:"the option's id, as defined by the template"`
	Value string `json:"value,omitempty" jsonschema:"the option's display value"`
}

// New projects one assessment onto the tool-facing shape.
func New(a clients.Assessment) Assessment {
	out := Assessment{
		ID:                  a.ID,
		Name:                a.Name,
		Status:              a.Status,
		Template:            newTemplate(a.Template),
		Asset:               newRef(a.Asset),
		AssessmentReview:    newRef(a.AssessmentReview),
		Owner:               newRef(a.Owner),
		IsVisibleToEveryone: a.IsVisibleToEveryone,
		CreatedOn:           a.CreatedOn,
		LastModifiedOn:      a.LastModifiedOn,
		SubmittedOn:         a.SubmittedOn,
	}
	for _, q := range a.Content {
		out.Content = append(out.Content, QuestionAndAnswer{
			ID:          q.ID,
			Name:        q.Name,
			Description: q.Description,
			Answer:      newAnswer(q.Answer),
			Comments:    q.Comments,
		})
	}
	for _, s := range a.Assignees {
		out.Assignees = append(out.Assignees, Assignee{ID: s.ID, Type: s.Type})
	}
	return out
}

// NewPtr projects an optional assessment, keeping nil as nil.
func NewPtr(a *clients.Assessment) *Assessment {
	if a == nil {
		return nil
	}
	out := New(*a)
	return &out
}

// NewList projects a list of assessments.
func NewList(in []clients.Assessment) []Assessment {
	out := make([]Assessment, 0, len(in))
	for _, a := range in {
		out = append(out, New(a))
	}
	return out
}

func newRef(r *clients.AssessmentRef) *Ref {
	if r == nil {
		return nil
	}
	return &Ref{ID: r.ID, Name: r.Name}
}

func newTemplate(t *clients.AssessmentTemplate) *Template {
	if t == nil {
		return nil
	}
	return &Template{
		ID:        t.ID,
		Name:      t.Name,
		Version:   string(t.Version),
		Status:    t.Status,
		AssetType: newRef(t.AssetType),
	}
}

func newAnswer(a *clients.Answer) *Answer {
	if a == nil {
		return nil
	}
	if items, ok := asItems(a.Value); ok {
		return &Answer{Type: a.Type, Items: items}
	}
	return &Answer{Type: a.Type, Value: asString(a.Value)}
}

// asItems recognises the [{id, value}] shape of a choice answer. It reports
// false for any other list, which then falls back to a string value, so no
// answer the API sends is dropped.
func asItems(v any) ([]Item, bool) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	items := make([]Item, 0, len(list))
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, false
		}
		id, ok := m["id"].(string)
		if !ok || id == "" {
			return nil, false
		}
		items = append(items, Item{ID: id, Value: asString(m["value"])})
	}
	return items, true
}

// asString renders an untyped answer value as a string. A value that is not a
// JSON scalar keeps its JSON form, so it stays readable and round-trips.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
