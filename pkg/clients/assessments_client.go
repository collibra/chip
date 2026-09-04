package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Assessments API v2 (/rest/assessments/v2) — the Assessments application's own
// REST API. Assessment "conduct" objects are NOT catalog assets, so they are
// reached here rather than through /rest/2.0. Served on the same host as the
// DGC API, so the shared RoundTripper injects host + auth transparently.
const assessmentsAPIBasePath = "/rest/assessments/v2"

// Assessment is a conducted assessment. Only the fields chip reads or writes
// are modelled; unknown fields are ignored on decode.
type Assessment struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name,omitempty"`
	Status              string              `json:"status,omitempty"` // DRAFT | SUBMITTED | OBSOLETE
	Template            *AssessmentTemplate `json:"template,omitempty"`
	Content             []QuestionAndAnswer `json:"content,omitempty"`
	Asset               *AssessmentRef      `json:"asset,omitempty"`
	AssessmentReview    *AssessmentRef      `json:"assessmentReview,omitempty"`
	Assignees           []Assignee          `json:"assignees,omitempty"`
	Owner               *AssessmentRef      `json:"owner,omitempty"`
	IsVisibleToEveryone *bool               `json:"isVisibleToEveryone,omitempty"`
	CreatedOn           string              `json:"createdOn,omitempty"`
	LastModifiedOn      string              `json:"lastModifiedOn,omitempty"`
	SubmittedOn         string              `json:"submittedOn,omitempty"`
}

// AssessmentRef is the {id, name?} shape the API uses for assets, reviews,
// owners, users, and templates references.
type AssessmentRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Assignee is a user or group assigned to an assessment.
type Assignee struct {
	ID   string `json:"id"`
	Type string `json:"type"` // USER | GROUP
}

// AssessmentTemplate is the (thin) template metadata. It does NOT expose the
// template's questions — those come from an assessment's Content.
type AssessmentTemplate struct {
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	Version   VersionString  `json:"version,omitempty"`
	Status    string         `json:"status,omitempty"` // DRAFT | PUBLISHED | OBSOLETE
	AssetType *AssessmentRef `json:"assetType,omitempty"`
}

// VersionString holds a template version, which the API sends as a JSON number
// (e.g. 8) but occasionally as a string. It is a string type so the generated
// output schema ("string") matches the marshaled value; UnmarshalJSON accepts
// either form. Using json.Number here caused a schema/wire mismatch — the
// schema said "string" (json.Number's underlying kind) while it marshaled as a
// bare number, failing output validation.
type VersionString string

func (v *VersionString) UnmarshalJSON(data []byte) error {
	*v = VersionString(strings.Trim(string(data), `"`))
	return nil
}

// QuestionAndAnswer is one question plus its current answer, as returned by GET.
type QuestionAndAnswer struct {
	ID          string  `json:"id"`
	Name        string  `json:"name,omitempty"`
	Description string  `json:"description,omitempty"`
	Answer      *Answer `json:"answer,omitempty"`
	Comments    string  `json:"comments,omitempty"`
}

// Answer is a typed answer as returned by GET. The API sends the value in a
// shape that depends on Type (TEXT/HTML/EXPRESSION/DATE → string, NUMBER →
// number, BOOLEAN → bool, ITEMS/ASSETS/USERORGROUPS/ATTACHMENTS → arrays), so
// the wire form is untyped. UnmarshalJSON narrows it onto concrete fields: a
// scalar onto Value, a list of options onto Items.
//
// The fields must stay concrete. A tool schema is generated from these types,
// and an untyped field makes the reflector emit a subschema with no type at
// all, which strict MCP clients reject — one such tool can fail a client's
// whole tool import.
type Answer struct {
	Type  string       `json:"type"`
	Value string       `json:"value,omitempty"`
	Items []AnswerItem `json:"items,omitempty"`
}

// AnswerItem is one selected option of a choice (ITEMS) answer.
type AnswerItem struct {
	ID    string `json:"id"`
	Value string `json:"value,omitempty"`
}

func (a *Answer) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type  string `json:"type"`
		Value any    `json:"value,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Type = raw.Type
	if items, ok := answerItems(raw.Value); ok {
		a.Items = items
		return nil
	}
	a.Value = answerString(raw.Value)
	return nil
}

// answerItems recognises the [{id, value}] shape of a choice answer. It reports
// false for any other list, which then falls back to a string value, so no
// answer the API sends is dropped.
func answerItems(v any) ([]AnswerItem, bool) {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return nil, false
	}
	items := make([]AnswerItem, 0, len(list))
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			return nil, false
		}
		id, ok := m["id"].(string)
		if !ok || id == "" {
			return nil, false
		}
		items = append(items, AnswerItem{ID: id, Value: answerString(m["value"])})
	}
	return items, true
}

// answerString renders an untyped answer value as a string. A value that is not
// a JSON scalar keeps its JSON form, so it stays readable and round-trips.
func answerString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
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

// AnswerInput is the write shape of an answer. Value stays untyped because the
// API expects a different JSON type per answer type, and the tool layer builds
// and validates it. It is never part of a tool schema — only Answer is.
type AnswerInput struct {
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
}

// QuestionIDAndAnswer is the write shape: a question id and the answer to set.
type QuestionIDAndAnswer struct {
	ID       string       `json:"id"`
	Answer   *AnswerInput `json:"answer,omitempty"`
	Comments *string      `json:"comments,omitempty"`
}

// UpdateAssessmentRequest is the PATCH body. All fields optional (partial
// update); nil/omitted fields are left unchanged.
type UpdateAssessmentRequest struct {
	Name                   *string               `json:"name,omitempty"`
	Status                 *string               `json:"status,omitempty"`
	Owner                  *AssessmentRef        `json:"owner,omitempty"`
	Assignees              []Assignee            `json:"assignees,omitempty"`
	Content                []QuestionIDAndAnswer `json:"content,omitempty"`
	IsVisibleToEveryone    *bool                 `json:"isVisibleToEveryone,omitempty"`
	AssessmentReviewDomain *AssessmentRef        `json:"assessmentReviewDomain,omitempty"`
}

// CreateAssessmentRequest is the POST body. Only Template is required; provide
// at least a Name or an Asset.
type CreateAssessmentRequest struct {
	Template               AssessmentRef         `json:"template"`
	Name                   string                `json:"name,omitempty"`
	Asset                  *AssessmentRef        `json:"asset,omitempty"`
	Assignees              []Assignee            `json:"assignees,omitempty"`
	Content                []QuestionIDAndAnswer `json:"content,omitempty"`
	Owner                  *AssessmentRef        `json:"owner,omitempty"`
	Status                 *string               `json:"status,omitempty"`
	IsVisibleToEveryone    *bool                 `json:"isVisibleToEveryone,omitempty"`
	AssessmentReviewDomain *AssessmentRef        `json:"assessmentReviewDomain,omitempty"`
}

// ListAssessmentsParams are the query filters for listing assessments. All are
// optional and combine.
type ListAssessmentsParams struct {
	Name             string `url:"name,omitempty"`
	Status           string `url:"status,omitempty"`
	TemplateID       string `url:"templateId,omitempty"`
	TemplateVersion  string `url:"templateVersion,omitempty"`
	AssetID          string `url:"assetId,omitempty"`
	LastModifiedFrom string `url:"lastModifiedFrom,omitempty"`
	LastModifiedTo   string `url:"lastModifiedTo,omitempty"`
	Limit            int    `url:"limit,omitempty"`
	Cursor           string `url:"cursor,omitempty"`
}

// PagedAssessments is a cursor-paged list of assessments.
type PagedAssessments struct {
	NextCursor string       `json:"nextCursor,omitempty"`
	Results    []Assessment `json:"results"`
}

// ListTemplatesParams are the query filters for listing templates.
type ListTemplatesParams struct {
	Name              string `url:"name,omitempty"`
	Status            string `url:"status,omitempty"`
	AssetTypeID       string `url:"assetTypeId,omitempty"`
	LatestVersionOnly *bool  `url:"latestVersionOnly,omitempty"`
	Limit             int    `url:"limit,omitempty"`
	Cursor            string `url:"cursor,omitempty"`
}

// PagedTemplates is a cursor-paged list of templates.
type PagedTemplates struct {
	NextCursor string               `json:"nextCursor,omitempty"`
	Results    []AssessmentTemplate `json:"results"`
}

// GetAssessment retrieves an assessment by its assessment ID.
func GetAssessment(ctx context.Context, client *http.Client, id string) (*Assessment, error) {
	endpoint := fmt.Sprintf("%s/assessments/%s", assessmentsAPIBasePath, url.PathEscape(id))
	return getAssessment(ctx, client, endpoint)
}

// GetAssessmentByReview retrieves an assessment by its Assessment Review asset
// UUID — the bridge from the catalog side to an assessment.
func GetAssessmentByReview(ctx context.Context, client *http.Client, reviewAssetID string) (*Assessment, error) {
	endpoint := fmt.Sprintf("%s/assessments/by/assessmentReview/%s", assessmentsAPIBasePath, url.PathEscape(reviewAssetID))
	return getAssessment(ctx, client, endpoint)
}

func getAssessment(ctx context.Context, client *http.Client, endpoint string) (*Assessment, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("get assessment: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	body, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}

	var assessment Assessment
	if err := json.Unmarshal(body, &assessment); err != nil {
		return nil, fmt.Errorf("get assessment: decoding response: %w", err)
	}
	return &assessment, nil
}

// ListAssessments lists assessments matching the given filters. Used to resolve
// an assessment from the asset it was conducted on (AssetID filter).
func ListAssessments(ctx context.Context, client *http.Client, params ListAssessmentsParams) (*PagedAssessments, error) {
	endpoint, err := buildUrl(assessmentsAPIBasePath+"/assessments", params)
	if err != nil {
		return nil, fmt.Errorf("list assessments: building url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("list assessments: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	body, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var paged PagedAssessments
	if err := json.Unmarshal(body, &paged); err != nil {
		return nil, fmt.Errorf("list assessments: decoding response: %w", err)
	}
	return &paged, nil
}

// UpdateAssessment applies a partial update to an assessment (PATCH) and
// returns the updated assessment.
func UpdateAssessment(ctx context.Context, client *http.Client, id string, request UpdateAssessmentRequest) (*Assessment, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("update assessment: marshaling request: %w", err)
	}
	endpoint := fmt.Sprintf("%s/assessments/%s", assessmentsAPIBasePath, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("update assessment: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	respBody, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var assessment Assessment
	if err := json.Unmarshal(respBody, &assessment); err != nil {
		return nil, fmt.Errorf("update assessment: decoding response: %w", err)
	}
	return &assessment, nil
}

// CreateAssessment creates a new assessment from a template (POST) and returns
// the created assessment (including its questions in Content).
func CreateAssessment(ctx context.Context, client *http.Client, request CreateAssessmentRequest) (*Assessment, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("create assessment: marshaling request: %w", err)
	}
	endpoint := assessmentsAPIBasePath + "/assessments"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create assessment: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	respBody, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var assessment Assessment
	if err := json.Unmarshal(respBody, &assessment); err != nil {
		return nil, fmt.Errorf("create assessment: decoding response: %w", err)
	}
	return &assessment, nil
}

// ListTemplates lists assessment templates matching the given filters.
func ListTemplates(ctx context.Context, client *http.Client, params ListTemplatesParams) (*PagedTemplates, error) {
	endpoint, err := buildUrl(assessmentsAPIBasePath+"/templates", params)
	if err != nil {
		return nil, fmt.Errorf("list templates: building url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("list templates: building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	body, err := executeCollibraRequest(client, req)
	if err != nil {
		return nil, err
	}
	var paged PagedTemplates
	if err := json.Unmarshal(body, &paged); err != nil {
		return nil, fmt.Errorf("list templates: decoding response: %w", err)
	}
	return &paged, nil
}
