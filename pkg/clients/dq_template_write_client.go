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
	// ErrDQRuleNotFound means the job has no rule under the given name. The
	// detach endpoint answers 404 for this, distinctly from an unknown template.
	ErrDQRuleNotFound = errors.New("rule not found on job")
	// ErrDQRuleNotFromTemplate means the rule exists but is not currently linked
	// to the given template — either it is already standalone, or it came from a
	// different template. The API answers 400 and does not distinguish the two.
	ErrDQRuleNotFromTemplate = errors.New("rule is not linked to this template")
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

// dqRuleDetachRequest is the detach payload (RuleTemplateDetachRequest). A
// deployment has no id of its own, so the rule is addressed by the job it runs
// on plus its deployed name.
type dqRuleDetachRequest struct {
	JobName          string `json:"jobName"`
	DeployedRuleName string `json:"deployedRuleName"`
}

// DetachDQRuleFromTemplate soft-unlinks one deployed rule from its template —
// POST /rest/dq/1.0/ruleTemplates/{ruleTemplateName}/detach. The rule and all of
// its run history are preserved; only the link is cleared, so later cascades
// from the template no longer reach it. Answers 204 with no body on success.
//
// The 400 is deliberately mapped to a single sentinel: RuleTemplatesBll.detach
// raises the same RULE_NOT_FROM_TEMPLATE for a rule that is already standalone
// and for one that belongs to a different template, so the API gives callers no
// way to tell those apart.
func DetachDQRuleFromTemplate(ctx context.Context, client *http.Client, ruleTemplateName, jobName, deployedRuleName string) error {
	path := "/rest/dq/1.0/ruleTemplates/" + url.PathEscape(ruleTemplateName) + "/detach"
	respBody, status, err := dqDo(ctx, client, http.MethodPost, path, dqRuleDetachRequest{
		JobName:          jobName,
		DeployedRuleName: deployedRuleName,
	})
	if err != nil {
		return fmt.Errorf("detaching dq rule from template: %w", err)
	}
	switch status {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusBadRequest:
		return fmt.Errorf("detaching dq rule from template: %w: %q on job %q: %s",
			ErrDQRuleNotFromTemplate, deployedRuleName, jobName, string(respBody))
	case http.StatusNotFound:
		// The endpoint answers 404 for both an unknown template and a rule that
		// does not exist on the job; the body is the only discriminator, so the
		// tool layer reports both possibilities rather than guessing.
		return fmt.Errorf("detaching dq rule from template: %w: template %q or rule %q on job %q: %s",
			ErrDQRuleNotFound, ruleTemplateName, deployedRuleName, jobName, string(respBody))
	case http.StatusForbidden:
		return fmt.Errorf("detaching dq rule from template: missing permission to manage template deployments: %s", string(respBody))
	default:
		return fmt.Errorf("detaching dq rule from template: unexpected status %d: %s", status, string(respBody))
	}
}
