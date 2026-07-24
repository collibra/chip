// Package edit_assessment implements the edit_assessment MCP tool: a single
// entry point for editing a conducted assessment (answers, status, name, owner,
// assignees, visibility) via a typed list of operations. Assessments are NOT
// catalog assets — they live in the Assessments application's own REST API,
// which takes ONE partial PATCH. So unlike edit_asset (many API calls), this
// tool validates every operation up front, collects them into a single
// UpdateAssessmentRequest, and makes ONE UpdateAssessment call: the PATCH is
// atomic, so any validation failure aborts the whole request.
package edit_assessment

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OperationType enumerates the edits edit_assessment can perform.
type OperationType string

const (
	OpSetAnswer     OperationType = "set_answer"
	OpSetStatus     OperationType = "set_status"
	OpSetName       OperationType = "set_name"
	OpSetOwner      OperationType = "set_owner"
	OpSetAssignees  OperationType = "set_assignees"
	OpSetVisibility OperationType = "set_visibility"
)

// Valid assessment statuses.
const (
	StatusDraft     = "DRAFT"
	StatusSubmitted = "SUBMITTED"
	StatusObsolete  = "OBSOLETE"
)

// Input is the tool's typed input.
type Input struct {
	Assessment string      `json:"assessment" jsonschema:"Required. The conducted assessment to edit — its name (resolved to a single match; ambiguous names return the candidates) or its UUID."`
	Operations []Operation `json:"operations" jsonschema:"Required. Non-empty list of operations to apply. The whole request is a single atomic PATCH, so if any operation fails validation none are applied. Each operation's type selects which additional fields are used (see Operation)."`
}

// Operation is a discriminated union: the 'type' field selects which other
// fields are interpreted. Unused fields are ignored.
type Operation struct {
	Type OperationType `json:"type" jsonschema:"Required. One of: set_answer, set_status, set_name, set_owner, set_assignees, set_visibility."`

	// set_answer
	QuestionID string      `json:"questionId,omitempty" jsonschema:"For set_answer: the id of a question in the assessment's content."`
	Comments   string      `json:"comments,omitempty" jsonschema:"For set_answer: optional free-text comment to attach to the answer."`
	AnswerType string      `json:"answerType,omitempty" jsonschema:"For set_answer: the answer type — TEXT, HTML, EXPRESSION, NUMBER, BOOLEAN, DATE, or ITEMS. Required when the question has not been answered yet (a blank question carries no type). Optional when the question already has an answer, in which case the existing type is used."`
	Items      []ItemInput `json:"items,omitempty" jsonschema:"For set_answer when the answer type is ITEMS (a choice question): the selected option(s). Provide these instead of 'value'. Each item's id must match an option defined by the template."`

	// set_answer (scalar types) / set_status / set_name / set_visibility use value.
	Value string `json:"value,omitempty" jsonschema:"For set_answer with a scalar type: the value as a string (NUMBER must parse as a number, BOOLEAN as 'true'/'false', DATE as 'yyyy-MM-dd', TEXT/HTML/EXPRESSION passed through). For set_status: DRAFT, SUBMITTED, or OBSOLETE. For set_name: the new assessment name. For set_visibility: 'true' or 'false'. Not used for ITEMS answers (use 'items')."`

	// set_owner
	UserID string `json:"userId,omitempty" jsonschema:"For set_owner: UUID of the user to set as the assessment owner."`

	// set_assignees
	Assignees []AssigneeInput `json:"assignees,omitempty" jsonschema:"For set_assignees: the full list of assignees to set (replaces existing). Each has an id (UUID) and type (USER or GROUP)."`
}

// AssigneeInput is one user/group assignment in a set_assignees op.
type AssigneeInput struct {
	ID   string `json:"id" jsonschema:"UUID of the user or group."`
	Type string `json:"type" jsonschema:"USER or GROUP."`
}

// ItemInput is one selected option for an ITEMS (choice) answer.
type ItemInput struct {
	ID    string `json:"id" jsonschema:"The option's id, as defined by the template."`
	Value string `json:"value,omitempty" jsonschema:"The option's display value. Defaults to id if omitted."`
}

// answerItem is the wire shape of one ITEMS option ({id, value}).
type answerItem struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

// OutputStatus summarises the result of the call.
type OutputStatus string

const (
	StatusSuccess OutputStatus = "success"
	StatusError   OutputStatus = "error"
)

// Output is the tool's typed output. Mirrors edit_asset: an overall status,
// per-operation results, the updated assessment on success, and an Error string
// for a whole-request failure.
type Output struct {
	Status     OutputStatus        `json:"status" jsonschema:"Overall status: success if the PATCH applied, error if any operation failed validation or the API call failed. The PATCH is atomic — there is no partial success."`
	Results    []OperationResult   `json:"results" jsonschema:"Per-operation outcomes, in the same order as the input operations. On validation failure the failing operations carry an error message; on API failure every operation is marked error."`
	Assessment *clients.Assessment `json:"assessment,omitempty" jsonschema:"The assessment's state after the update. Present only on success."`
	Error      string              `json:"error,omitempty" jsonschema:"Populated when the overall request could not be completed (assessment not found, validation failure, or API/permission error). Per-operation detail lives in Results."`
}

// OperationResult is the outcome of a single operation.
type OperationResult struct {
	Operation  OperationType `json:"operation"`
	Status     string        `json:"status" jsonschema:"'success' or 'error'."`
	QuestionID string        `json:"questionId,omitempty"`
	Value      string        `json:"value,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "edit_assessment",
		Title: "Edit Assessment",
		Description: "Edit a conducted assessment (NOT a catalog asset — assessments live in the Assessments application) by submitting a list of typed operations against a single assessment (identified by name or UUID). " +
			"Supported operations: " +
			"set_answer (set a question's answer by questionId, with an optional comment); " +
			"set_status (DRAFT, SUBMITTED, or OBSOLETE); " +
			"set_name (rename the assessment); " +
			"set_owner (set the owner by user UUID); " +
			"set_assignees (replace the assignee list with the given users/groups); " +
			"set_visibility ('true'/'false' for whether the assessment is visible to everyone). " +
			"For set_answer, supported answer types are TEXT, HTML, EXPRESSION (value passed through), NUMBER (must parse), BOOLEAN ('true'/'false'), DATE ('yyyy-MM-dd') — all via 'value' — and ITEMS (choice questions) via 'items'. " +
			"If the question has not been answered yet, supply 'answerType'; if it already has an answer, the existing type is used. " +
			"Answer types ASSETS, USERORGROUPS, and ATTACHMENTS are not yet supported and return a per-operation error. " +
			"The whole request is applied as a single atomic PATCH: every operation is validated first and if any fails, none are applied.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(true), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Phase 0: input-validation problems → Go error. Cheap local checks first.
		if len(input.Operations) == 0 {
			return Output{}, fmt.Errorf("operations must not be empty")
		}
		for _, op := range input.Operations {
			switch op.Type {
			case OpSetAnswer, OpSetStatus, OpSetName, OpSetOwner, OpSetAssignees, OpSetVisibility:
			default:
				return Output{}, fmt.Errorf("unknown operation type %q", op.Type)
			}
		}

		// Resolve the assessment reference (name or UUID) to a single id.
		assessmentID, err := resolveAssessmentID(ctx, collibraClient, input.Assessment)
		if err != nil {
			return Output{}, err
		}

		// Phase 1: fetch the assessment once — source of truth for question types.
		assessment, err := clients.GetAssessment(ctx, collibraClient, assessmentID)
		if err != nil {
			return Output{Status: StatusError, Error: fmt.Sprintf("assessment %s not found or unreadable: %s", assessmentID, err.Error())}, nil
		}

		// Index questions by id so set_answer can look up the answer type.
		answerTypeByQuestion := make(map[string]string, len(assessment.Content))
		for _, q := range assessment.Content {
			t := ""
			if q.Answer != nil {
				t = q.Answer.Type
			}
			answerTypeByQuestion[q.ID] = t
		}

		// Phase 2: validate every operation, building the single PATCH as we go.
		// Any failure aborts before the PATCH (it's all-or-nothing).
		results := make([]OperationResult, len(input.Operations))
		req := clients.UpdateAssessmentRequest{}
		failed := false

		for i, op := range input.Operations {
			res := OperationResult{Operation: op.Type, Status: "success"}
			switch op.Type {
			case OpSetAnswer:
				res.QuestionID = op.QuestionID
				res.Value = op.Value
				existingType, ok := answerTypeByQuestion[op.QuestionID]
				if !ok {
					res.Status, res.Error = "error", fmt.Sprintf("questionId %q not found in assessment content", op.QuestionID)
					break
				}
				// A blank question carries no type, so fall back to the
				// caller-supplied answerType; a previously-answered question's
				// existing type wins.
				answerType, terr := resolveAnswerType(existingType, op.AnswerType)
				if terr != nil {
					res.Status, res.Error = "error", terr.Error()
					break
				}
				value, verr := buildAnswer(answerType, op.Value, op.Items)
				if verr != nil {
					res.Status, res.Error = "error", verr.Error()
					break
				}
				entry := clients.QuestionIDAndAnswer{
					ID:     op.QuestionID,
					Answer: &clients.Answer{Type: answerType, Value: value},
				}
				if op.Comments != "" {
					entry.Comments = chip.Ptr(op.Comments)
				}
				req.Content = append(req.Content, entry)

			case OpSetStatus:
				res.Value = op.Value
				status := strings.ToUpper(strings.TrimSpace(op.Value))
				if status != StatusDraft && status != StatusSubmitted && status != StatusObsolete {
					res.Status, res.Error = "error", fmt.Sprintf("status must be one of DRAFT, SUBMITTED, OBSOLETE; got %q", op.Value)
					break
				}
				req.Status = chip.Ptr(status)

			case OpSetName:
				res.Value = op.Value
				if strings.TrimSpace(op.Value) == "" {
					res.Status, res.Error = "error", "name must not be empty"
					break
				}
				req.Name = chip.Ptr(op.Value)

			case OpSetOwner:
				res.Value = op.UserID
				if err := validation.UUID("userId", op.UserID); err != nil {
					res.Status, res.Error = "error", err.Error()
					break
				}
				req.Owner = &clients.AssessmentRef{ID: op.UserID}

			case OpSetAssignees:
				assignees := make([]clients.Assignee, 0, len(op.Assignees))
				var aerr error
				for _, a := range op.Assignees {
					if err := validation.UUID("assignee id", a.ID); err != nil {
						aerr = err
						break
					}
					t := strings.ToUpper(strings.TrimSpace(a.Type))
					if t != "USER" && t != "GROUP" {
						aerr = fmt.Errorf("assignee type must be USER or GROUP; got %q", a.Type)
						break
					}
					assignees = append(assignees, clients.Assignee{ID: a.ID, Type: t})
				}
				if aerr != nil {
					res.Status, res.Error = "error", aerr.Error()
					break
				}
				req.Assignees = assignees

			case OpSetVisibility:
				res.Value = op.Value
				b, berr := strconv.ParseBool(strings.TrimSpace(op.Value))
				if berr != nil {
					res.Status, res.Error = "error", fmt.Sprintf("visibility value must be true or false; got %q", op.Value)
					break
				}
				req.IsVisibleToEveryone = chip.Ptr(b)
			}

			if res.Status == "error" {
				failed = true
			}
			results[i] = res
		}

		// Phase 3: if any op failed validation, abort — the PATCH is atomic.
		if failed {
			return Output{
				Status:  StatusError,
				Results: results,
				Error:   "one or more operations failed validation; no changes were applied (the assessment PATCH is atomic)",
			}, nil
		}

		// Phase 4: single PATCH.
		updated, err := clients.UpdateAssessment(ctx, collibraClient, assessmentID, req)
		if err != nil {
			for i := range results {
				results[i].Status = "error"
			}
			return Output{
				Status:  StatusError,
				Results: results,
				Error:   fmt.Sprintf("failed to update assessment: %s", err.Error()),
			}, nil
		}

		return Output{
			Status:     StatusSuccess,
			Results:    results,
			Assessment: updated,
		}, nil
	}
}

// resolveAssessmentID turns the caller's assessment reference into a single
// assessment UUID. A UUID is used as-is; a name is resolved (case-insensitive,
// exact match preferred) and must land on exactly one assessment, else a
// self-correcting error lists the candidates.
func resolveAssessmentID(ctx context.Context, client *http.Client, ref string) (string, error) {
	r := strings.TrimSpace(ref)
	if r == "" {
		return "", fmt.Errorf("assessment is required (its name or UUID)")
	}
	if validation.UUID("assessment", r) == nil {
		return r, nil
	}

	paged, err := clients.ListAssessments(ctx, client, clients.ListAssessmentsParams{Name: r, Limit: 50})
	if err != nil {
		return "", fmt.Errorf("looking up assessment %q: %w", r, err)
	}
	// Prefer exact (case-insensitive) name matches; fall back to the API's
	// contains-matches.
	var matches []clients.Assessment
	for _, a := range paged.Results {
		if strings.EqualFold(strings.TrimSpace(a.Name), r) {
			matches = append(matches, a)
		}
	}
	if len(matches) == 0 {
		matches = paged.Results
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no assessment found matching name %q", ref)
	case 1:
		return matches[0].ID, nil
	}
	lines := make([]string, 0, len(matches))
	for _, a := range matches {
		lines = append(lines, fmt.Sprintf("%q (%s, id %s)", a.Name, a.Status, a.ID))
	}
	sort.Strings(lines)
	return "", fmt.Errorf("assessment name %q is ambiguous; matched %d — %s. Specify the exact name or the assessment UUID", ref, len(matches), strings.Join(lines, "; "))
}

// resolveAnswerType picks the answer type for a set_answer op. A previously
// answered question already carries its type (existingType), which is
// authoritative. A blank question carries none, so the caller must supply one
// via the op's answerType. Pure, so it's unit-testable.
func resolveAnswerType(existingType, callerType string) (string, error) {
	if existingType != "" {
		return existingType, nil
	}
	if strings.TrimSpace(callerType) != "" {
		return strings.ToUpper(strings.TrimSpace(callerType)), nil
	}
	return "", fmt.Errorf("question has no answer yet; specify answerType (e.g. TEXT, HTML, NUMBER, BOOLEAN, DATE, ITEMS)")
}

// buildAnswer builds the Go value for an answer of the given type, per the
// Assessments API's per-type shape. Scalar types read from value; ITEMS reads
// from items. Pure (no HTTP, no state) so the rules are unit-testable.
//
// Supported: TEXT/HTML/EXPRESSION → string, NUMBER → float64, BOOLEAN → bool,
// DATE → "yyyy-MM-dd" string (format checked), ITEMS → []{id,value}.
// Still deferred: ASSETS/USERORGROUPS/ATTACHMENTS (asset/user/attachment refs).
func buildAnswer(answerType, value string, items []ItemInput) (any, error) {
	switch strings.ToUpper(answerType) {
	case "TEXT", "HTML", "EXPRESSION":
		return value, nil
	case "NUMBER":
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return nil, fmt.Errorf("answer type NUMBER requires a numeric value; got %q", value)
		}
		return n, nil
	case "BOOLEAN":
		b, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("answer type BOOLEAN requires true or false; got %q", value)
		}
		return b, nil
	case "DATE":
		v := strings.TrimSpace(value)
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return nil, fmt.Errorf("answer type DATE requires yyyy-MM-dd; got %q", value)
		}
		return v, nil
	case "ITEMS":
		if len(items) == 0 {
			return nil, fmt.Errorf("answer type ITEMS requires at least one item (each with an id)")
		}
		built := make([]answerItem, len(items))
		for i, it := range items {
			if strings.TrimSpace(it.ID) == "" {
				return nil, fmt.Errorf("ITEMS answer: item %d is missing an id", i)
			}
			v := it.Value
			if v == "" {
				v = it.ID
			}
			built[i] = answerItem{ID: it.ID, Value: v}
		}
		return built, nil
	case "ASSETS", "USERORGROUPS", "ATTACHMENTS":
		return nil, fmt.Errorf("answer type %s is not yet supported by edit_assessment", strings.ToUpper(answerType))
	case "":
		return nil, fmt.Errorf("question has no answer type; cannot set an answer")
	default:
		return nil, fmt.Errorf("unknown answer type %q", answerType)
	}
}
