package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	agentContractsPath         = "/rest/aiGovernance/v1/agentContracts"
	agentContractVersionsPath  = "/rest/aiGovernance/v1/agentContractVersions"
	domainTypesPath            = "/rest/2.0/domainTypes"
	domainsPath                = "/rest/2.0/domains"
	governanceAssetDomainType  = "Governance Asset Domain"
	agentContractErrFeatureOff = "agentContractsFeatureDisabled"
)

// AgentContract is one agent contract as the Agent Contracts API returns it.
type AgentContract struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	CreatedOn           string `json:"createdOn"`
	LastModifiedOn      string `json:"lastModifiedOn"`
	LastModifiedBy      string `json:"lastModifiedBy"`
	Layer               string `json:"layer"`
	ActiveVersionID     string `json:"activeVersionId"`
	ActiveVersionNumber int    `json:"activeVersionNumber"`
	RuleCount           int    `json:"ruleCount"`
	GovernedAgentCount  *int   `json:"governedAgentCount"`
}

// AgentContractPage is one page of agent contracts.
type AgentContractPage struct {
	Total   *int            `json:"total,omitempty"`
	AgentID string          `json:"agentId,omitempty"`
	Results []AgentContract `json:"results"`
}

// ListAgentContractsParams are the filters of the list call. Zero values are omitted.
type ListAgentContractsParams struct {
	Name         string
	AgentID      string
	SortField    string
	SortOrder    string
	Offset       int
	Limit        int
	IncludeTotal bool
}

// AgentContractReference identifies the contract a rule list belongs to.
type AgentContractReference struct {
	ID    string `json:"id"`
	Layer string `json:"layer"`
}

// AgentContractVersion is one version of an agent contract.
type AgentContractVersion struct {
	ID            string `json:"id"`
	CreatedOn     string `json:"createdOn"`
	CreatedBy     string `json:"createdBy"`
	VersionNumber int    `json:"versionNumber"`
}

// AgentContractRule is one natural language rule.
type AgentContractRule struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
	Rank      int    `json:"rank"`
}

// AgentContractRuleList is one version of a contract with all its rules.
type AgentContractRuleList struct {
	AgentContract        AgentContractReference `json:"agentContract"`
	AgentContractVersion AgentContractVersion   `json:"agentContractVersion"`
	Results              []AgentContractRule    `json:"results"`
}

// CreateAgentContractRequest is the body of the create call.
type CreateAgentContractRequest struct {
	Layer      string   `json:"layer"`
	Name       string   `json:"name"`
	DomainID   string   `json:"domainId,omitempty"`
	Statements []string `json:"statements"`
}

// GovernanceDomain is a domain in which an agent contract asset can be created.
type GovernanceDomain struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	CommunityName string `json:"communityName,omitempty"`
}

// ListAgentContracts reads one page of agent contracts.
func ListAgentContracts(ctx context.Context, c *http.Client, p ListAgentContractsParams) (*AgentContractPage, error) {
	slog.InfoContext(ctx, "Listing agent contracts")
	q := url.Values{}
	if p.Name != "" {
		q.Set("name", p.Name)
	}
	if p.AgentID != "" {
		q.Set("agentId", p.AgentID)
	}
	if p.SortField != "" {
		q.Set("sortField", p.SortField)
	}
	if p.SortOrder != "" {
		q.Set("sortOrder", p.SortOrder)
	}
	if p.Offset > 0 {
		q.Set("offset", strconv.Itoa(p.Offset))
	}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.IncludeTotal {
		q.Set("includeTotal", "true")
	}
	path := agentContractsPath
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var page AgentContractPage
	if err := agentContractJSON(ctx, c, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetAgentContract reads one agent contract.
func GetAgentContract(ctx context.Context, c *http.Client, id string) (*AgentContract, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading agent contract: %s", id))
	var contract AgentContract
	if err := agentContractJSON(ctx, c, http.MethodGet, agentContractsPath+"/"+url.PathEscape(id), nil, &contract); err != nil {
		return nil, err
	}
	return &contract, nil
}

// GetAgentContractRules reads the rules of a contract's active version.
func GetAgentContractRules(ctx context.Context, c *http.Client, contractID string) (*AgentContractRuleList, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading rules of agent contract: %s", contractID))
	var list AgentContractRuleList
	if err := agentContractJSON(ctx, c, http.MethodGet, agentContractsPath+"/"+url.PathEscape(contractID)+"/rules", nil, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// GetAgentContractVersionRules reads the rules of one exact version.
func GetAgentContractVersionRules(ctx context.Context, c *http.Client, versionID string) (*AgentContractRuleList, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading rules of agent contract version: %s", versionID))
	var list AgentContractRuleList
	if err := agentContractJSON(ctx, c, http.MethodGet, agentContractVersionsPath+"/"+url.PathEscape(versionID)+"/rules", nil, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// CreateAgentContract creates a contract and its first version. Sent once: a
// repeated create would fail with a name conflict or, worse, race another one.
func CreateAgentContract(ctx context.Context, c *http.Client, req CreateAgentContractRequest) (*AgentContract, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Creating agent contract: %s", req.Name))
	var contract AgentContract
	if err := agentContractJSON(ctx, c, http.MethodPost, agentContractsPath, req, &contract); err != nil {
		return nil, err
	}
	if contract.ID == "" {
		return nil, errors.New("Collibra answered success but the response did not contain a contract id")
	}
	return &contract, nil
}

// ReplaceAgentContractRules pushes the full ordered statement set as a new version.
func ReplaceAgentContractRules(ctx context.Context, c *http.Client, contractID string, statements []string) (*AgentContractRuleList, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Replacing rules of agent contract: %s", contractID))
	body := struct {
		Statements []string `json:"statements"`
	}{statements}
	var list AgentContractRuleList
	if err := agentContractJSON(ctx, c, http.MethodPut, agentContractsPath+"/"+url.PathEscape(contractID)+"/rules", body, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// ListGovernanceDomains lists the Governance Asset domains, the only domains a
// contract asset can be created in. name filters by name, ignoring case.
func ListGovernanceDomains(ctx context.Context, c *http.Client, name string, limit int) ([]GovernanceDomain, error) {
	slog.InfoContext(ctx, "Listing Governance Asset domains")
	if limit <= 0 {
		limit = 50
	}

	typeQuery := url.Values{"name": {governanceAssetDomainType}, "nameMatchMode": {"EXACT"}}
	var types struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := agentContractJSON(ctx, c, http.MethodGet, domainTypesPath+"?"+typeQuery.Encode(), nil, &types); err != nil {
		return nil, err
	}
	if len(types.Results) == 0 {
		return nil, nil
	}

	q := url.Values{"typeId": {types.Results[0].ID}, "limit": {strconv.Itoa(limit)}}
	if name != "" {
		q.Set("name", name)
		q.Set("nameMatchMode", "ANYWHERE")
	}
	var domains struct {
		Results []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Community struct {
				Name string `json:"name"`
			} `json:"community"`
		} `json:"results"`
	}
	if err := agentContractJSON(ctx, c, http.MethodGet, domainsPath+"?"+q.Encode(), nil, &domains); err != nil {
		return nil, err
	}
	out := make([]GovernanceDomain, 0, len(domains.Results))
	for _, d := range domains.Results {
		out = append(out, GovernanceDomain{ID: d.ID, Name: d.Name, CommunityName: d.Community.Name})
	}
	return out, nil
}

// agentContractJSON runs one request and decodes the JSON answer. It never
// retries: the caller decides, because writes are not safe to repeat blindly.
func agentContractJSON(ctx context.Context, c *http.Client, method, pathAndQuery string, payload, into any) error {
	var reader *bytes.Reader
	if payload != nil {
		encoded, err := marshalWithoutHTMLEscaping(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	var (
		req *http.Request
		err error
	)
	if reader != nil {
		req, err = http.NewRequestWithContext(ctx, method, pathAndQuery, reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, pathAndQuery, nil)
	}
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	body, status, err := executeRequestWithStatus(c, req)
	if err != nil {
		if status == 0 {
			return err
		}
		if status >= 300 && status < 400 {
			return signInRequiredError()
		}
		return parseAgentScanAPIError(status, body)
	}
	if err := json.Unmarshal(body, into); err != nil {
		if looksLikeHTML(body) {
			return signInRequiredError()
		}
		return fmt.Errorf("Collibra answered success but the response could not be read: %w", err)
	}
	return nil
}

// ContractFailure explains a failed Agent Contracts API call in plain
// language, with what to tell or ask the customer next.
func (e *AgentScanAPIError) ContractFailure() (message, guidance string) {
	if text, ok := contractErrorText[e.ErrorCode]; ok {
		message = text
	} else {
		switch e.StatusCode {
		case http.StatusUnauthorized:
			message = "Collibra did not accept the sign-in for this request (HTTP 401)."
		case http.StatusForbidden:
			message = "Collibra refused this request for the current user (HTTP 403)."
		case http.StatusNotFound:
			message = "Collibra does not know this agent contract, or it was deleted."
		default:
			message = fmt.Sprintf("Collibra could not complete the request (HTTP %d).", e.StatusCode)
		}
	}
	if e.UserMessage != "" {
		message += " Collibra said: " + e.UserMessage
	}

	switch {
	case e.ErrorCode == agentContractErrFeatureOff:
		guidance = "Do not retry. Tell the customer Agent Contracts is not turned on for their Collibra: it has to be enabled in AI Command Center first."
	case e.ErrorCode == "concurrentAgentContractUpdate":
		guidance = "Another change was applied first. Read the contract's rules again, show the customer what changed, and only push again after they agree to the updated version."
	case e.StatusCode == http.StatusUnauthorized:
		guidance = "Do not retry. Ask the customer to sign in to Collibra again, then start over."
	case e.StatusCode == http.StatusForbidden:
		guidance = "Do not retry or try another route: the same sign-in will be refused again. Tell the customer they lack the permission and to ask their Collibra administrator; creating contracts needs the global permission held by the AI Business User role, and changing one needs update permission on that contract."
	case e.StatusCode == http.StatusNotFound:
		guidance = "Nothing was changed. Ask the customer to confirm the contract, or list the contracts again."
	case e.StatusCode == http.StatusBadRequest || e.StatusCode == http.StatusConflict || e.StatusCode == http.StatusUnprocessableEntity:
		guidance = "Nothing was written. Explain the problem to the customer and fix the input with them before trying again."
	default:
		guidance = "It is not known whether anything was written. Read the contract back before trying again, and tell the customer it did not go through."
	}
	return message, guidance
}

var contractErrorText = map[string]string{
	"invalidRequest":                   "The request was malformed or broke a limit (a name or statement too long, no statements, or more than 200).",
	"invalidRuleStatement":             "A statement is blank after cleaning up its whitespace.",
	"duplicateRuleStatement":           "Two statements are the same after cleaning up their whitespace; every statement must be unique.",
	"domainNotFound":                   "The domain does not exist, or no domain was given.",
	"invalidDomainType":                "The domain is not a Governance Asset domain, which is the only kind a contract can be created in.",
	"globalAgentContractAlreadyExists": "A global agent contract already exists; only one is allowed.",
	"agentContractNameAlreadyInUse":    "That domain already contains an asset with this name.",
	"agentContractNotFound":            "Collibra does not know this agent contract, or it was deleted.",
	"agentContractVersionNotFound":     "Collibra does not know this agent contract version.",
	"agentContractNotViewable":         "The user cannot view every agent contract that governs this agent.",
	agentContractErrFeatureOff:         "The Agent Contracts feature is turned off for this Collibra.",
	"concurrentAgentContractUpdate":    "Another push to this contract was applied first.",
	"authorizationFailed":              "The user does not have the permission this action needs.",
	"authenticationFailed":             "Collibra did not accept the sign-in for this request.",
	AgentScanSignInRequiredCode:        "Collibra answered with a sign-in page, so the user is not signed in (or the session has expired).",
}

// DescribeContractError turns an error from the calls above into the pieces
// every contract tool reports.
func DescribeContractError(err error) (message, guidance, errorCode string, httpStatus int) {
	var apiErr *AgentScanAPIError
	if !errors.As(err, &apiErr) {
		return fmt.Sprintf("The request to Collibra did not complete: %v.", err),
			"This looks like a network problem rather than a rejection. For a read, trying once more is fine. For a write it is not known whether anything was written: read the contract back before trying again.",
			"", 0
	}
	message, guidance = apiErr.ContractFailure()
	return message, guidance, apiErr.ErrorCode, apiErr.StatusCode
}

const (
	maxContractStatements   = 200
	maxContractStatementLen = 2000
)

// NormalizeStatements applies the whitespace part of the API's normalization
// (trim, collapse runs of whitespace) and checks the API's limits, so a
// preview can show what will be stored and a bad input fails before any call.
// problem is empty when the statements are acceptable.
func NormalizeStatements(statements []string) (normalized []string, problem string) {
	if len(statements) == 0 {
		return nil, "statements is required: pass at least one rule statement."
	}
	if len(statements) > maxContractStatements {
		return nil, fmt.Sprintf("A contract holds at most %d statements; %d were given.", maxContractStatements, len(statements))
	}
	seen := make(map[string]int, len(statements))
	for i, raw := range statements {
		statement := strings.Join(strings.Fields(raw), " ")
		switch {
		case statement == "":
			return nil, fmt.Sprintf("Statement %d is empty or only whitespace.", i+1)
		case len([]rune(statement)) > maxContractStatementLen:
			return nil, fmt.Sprintf("Statement %d is longer than %d characters.", i+1, maxContractStatementLen)
		}
		if first, dup := seen[statement]; dup {
			return nil, fmt.Sprintf("Statements %d and %d are the same once whitespace is cleaned up; every statement must be unique.", first, i+1)
		}
		seen[statement] = i + 1
		normalized = append(normalized, statement)
	}
	return normalized, ""
}
