package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// Sentinel conditions callers need to tell apart from a generic failure when
// managing rule templates.
var (
	// ErrDQRuleTemplateNotFound means no template exists under the given name.
	ErrDQRuleTemplateNotFound = errors.New("rule template not found")
	// ErrDQRuleTemplateNameTaken means a template with that name already exists
	// (the create endpoint answers 409).
	ErrDQRuleTemplateNameTaken = errors.New("rule template name already taken")
	// ErrDQRuleTemplateReadOnly means the template is out-of-the-box
	// (system-defined) and the API refuses to modify or delete it.
	ErrDQRuleTemplateReadOnly = errors.New("rule template is out-of-the-box and cannot be modified")
)

// DQRuleTemplateWriteRequest is the create/update payload for a rule template
// (RuleTemplateWriteRequest).
//
// The public API requires ruleTemplateName, description, sql, dialect and at
// least one dimension on BOTH create and update — the update endpoint is a PUT
// (full replacement), not a PATCH. Callers wanting partial-update semantics must
// read the template first and merge, otherwise omitted fields are wiped.
type DQRuleTemplateWriteRequest struct {
	Name                 string   `json:"ruleTemplateName"`
	Description          string   `json:"description"`
	SQL                  string   `json:"sql"`
	Dialect              string   `json:"dialect"`
	Dimensions           []string `json:"dimensions"`
	Tolerance            *int     `json:"tolerance,omitempty"`
	BusinessRuleAssetIDs []string `json:"businessRuleAssetIds,omitempty"`
}

// CreateDQRuleTemplate creates a rule template —
// POST /rest/dq/1.0/ruleTemplates. Returns the created template with its
// server-assigned id.
func CreateDQRuleTemplate(ctx context.Context, client *http.Client, request DQRuleTemplateWriteRequest) (*DQRuleTemplate, error) {
	const op = "creating dq rule template"
	respBody, status, err := dqDo(ctx, client, http.MethodPost, "/rest/dq/1.0/ruleTemplates", request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if status != http.StatusCreated && status != http.StatusOK {
		switch status {
		case http.StatusConflict:
			return nil, fmt.Errorf("%s: %w: %q: %s", op, ErrDQRuleTemplateNameTaken, request.Name, string(respBody))
		case http.StatusBadRequest:
			return nil, fmt.Errorf("%s: rejected by the data quality service (invalid input, SQL that cannot be translated, or a businessRuleAssetIds entry that is not a Business Rule asset): %s", op, string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("%s: missing permission to manage rule templates: %s", op, string(respBody))
		default:
			return nil, fmt.Errorf("%s: unexpected status %d: %s", op, status, string(respBody))
		}
	}
	var created DQRuleTemplate
	if err := json.Unmarshal(respBody, &created); err != nil {
		return nil, fmt.Errorf("%s: decoding response: %w", op, err)
	}
	return &created, nil
}

// DQRuleTemplateUpdateResult is the update response (RuleTemplateUpdateResult):
// the updated template plus one outcome per rule the change cascaded onto, so a
// partial cascade (some deployments SKIPPED or FAILED) is visible to the caller.
type DQRuleTemplateUpdateResult struct {
	RuleTemplate DQRuleTemplate            `json:"ruleTemplate"`
	Deployments  []DQTemplateDeployOutcome `json:"deployments"`
}

// UpdateDQRuleTemplate replaces a rule template —
// PUT /rest/dq/1.0/ruleTemplates/{ruleTemplateName}.
//
// The change ALWAYS cascades to every rule deployed from the template, in a
// single transaction; the API exposes no way to update the definition alone. The
// returned per-deployment outcomes report which rules took the change.
func UpdateDQRuleTemplate(ctx context.Context, client *http.Client, ruleTemplateName string, request DQRuleTemplateWriteRequest) (*DQRuleTemplateUpdateResult, error) {
	const op = "updating dq rule template"
	path := "/rest/dq/1.0/ruleTemplates/" + url.PathEscape(ruleTemplateName)
	respBody, status, err := dqDo(ctx, client, http.MethodPut, path, request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusNotFound:
			return nil, fmt.Errorf("%s: %w: %q: %s", op, ErrDQRuleTemplateNotFound, ruleTemplateName, string(respBody))
		case http.StatusBadRequest:
			return nil, fmt.Errorf("%s: rejected by the data quality service (invalid input, SQL that cannot be translated, a name that is already taken, or an out-of-the-box template): %s", op, string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("%s: missing permission to manage rule templates: %s", op, string(respBody))
		default:
			return nil, fmt.Errorf("%s: unexpected status %d: %s", op, status, string(respBody))
		}
	}
	var result DQRuleTemplateUpdateResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("%s: decoding response: %w", op, err)
	}
	return &result, nil
}

// DeleteDQRuleTemplate deletes a rule template —
// DELETE /rest/dq/1.0/ruleTemplates/{ruleTemplateName}. When deleteDeployments
// is true every rule deployed from the template is deleted with it.
//
// The endpoint is idempotent: it answers 204 even when no template matched, so
// callers that need to report "no such template" must check for it beforehand.
func DeleteDQRuleTemplate(ctx context.Context, client *http.Client, ruleTemplateName string, deleteDeployments bool) error {
	const op = "deleting dq rule template"
	path := "/rest/dq/1.0/ruleTemplates/" + url.PathEscape(ruleTemplateName)
	if deleteDeployments {
		path += "?deleteDeployments=" + strconv.FormatBool(true)
	}
	respBody, status, err := dqDo(ctx, client, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	switch status {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusBadRequest:
		return fmt.Errorf("%s: %w: %q: %s", op, ErrDQRuleTemplateReadOnly, ruleTemplateName, string(respBody))
	case http.StatusForbidden:
		return fmt.Errorf("%s: missing permission to manage rule templates: %s", op, string(respBody))
	default:
		return fmt.Errorf("%s: unexpected status %d: %s", op, status, string(respBody))
	}
}
