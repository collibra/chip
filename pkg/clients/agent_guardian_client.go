package clients

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

const (
	agentOverviewFmt          = "/rest/guardianOverview/v1/aiAgents/%s"
	agentCompileFmt           = "/rest/guardianEnforcement/v1/bundles/by/aiAgent/%s/compile"
	agentCompilationReportFmt = "/rest/guardianEnforcement/v1/compilationReports/by/aiAgent/%s"
	agentLatestBundleFmt      = "/rest/guardianEnforcement/v1/bundles/by/aiAgent/%s"
	bundleFileFmt             = "/rest/guardianEnforcement/v1/bundles/%s/file"
)

// AgentGuardianOverview is one AI agent's state in each Guardian stage, as the
// Overview API returns it. Every block is a pointer so a block an older
// Collibra does not send can be told apart from an empty one.
type AgentGuardianOverview struct {
	AIAgent     *GuardianAgentRef            `json:"aiAgent,omitempty"`
	Fingerprint *GuardianFingerprintOverview `json:"fingerprint,omitempty"`
	Contracts   *[]GuardianGoverningContract `json:"contracts,omitempty"`
	Compilation *GuardianCompilationOverview `json:"compilation,omitempty"`
	Enforcement *GuardianEnforcementOverview `json:"enforcement,omitempty"`
	Collection  *GuardianCollectionOverview  `json:"collection,omitempty"`
}

// GuardianAgentRef is an AI agent or AI agent version, with its display name.
type GuardianAgentRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// GuardianFingerprintOverview is the agent's latest fingerprint and its import.
type GuardianFingerprintOverview struct {
	Status         string            `json:"status"`
	ID             string            `json:"id"`
	CreatedOn      string            `json:"createdOn,omitempty"`
	LastReceivedOn string            `json:"lastReceivedOn,omitempty"`
	AIAgentVersion *GuardianAgentRef `json:"aiAgentVersion,omitempty"`
	FailureMessage string            `json:"failureMessage,omitempty"`
}

// GuardianGoverningContract is one agent contract that governs the agent.
type GuardianGoverningContract struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Layer           string `json:"layer"`
	ActiveVersionID string `json:"activeVersionId"`
}

// GuardianCompilationOverview is the compilation of the agent's active version.
type GuardianCompilationOverview struct {
	Status       string                  `json:"status"`
	StaleBecause []string                `json:"staleBecause,omitempty"`
	LatestJob    *GuardianCompilationJob `json:"latestJob,omitempty"`
	LatestBundle *GuardianBundleSummary  `json:"latestBundle,omitempty"`
}

// GuardianCompilationJob is a compilation run. The job's id is deliberately
// not decoded: it describes how Collibra works inside, and progress is
// followed through the overview instead.
type GuardianCompilationJob struct {
	State          string `json:"state"`
	CreatedOn      string `json:"createdOn,omitempty"`
	FailureMessage string `json:"failureMessage,omitempty"`
	BundleID       string `json:"bundleId,omitempty"`
}

// GuardianBundleSummary is the latest rules file of the agent's active version.
type GuardianBundleSummary struct {
	ID                 string   `json:"id"`
	Version            int64    `json:"version"`
	CompiledAt         string   `json:"compiledAt,omitempty"`
	AgentVersionID     string   `json:"agentVersionId,omitempty"`
	AgentFingerprintID string   `json:"agentFingerprintId,omitempty"`
	ContractVersionIDs []string `json:"contractVersionIds,omitempty"`
}

// GuardianEnforcementOverview is what telemetry observed for the agent.
type GuardianEnforcementOverview struct {
	Status                 string                  `json:"status"`
	ObservedAIAgentVersion *GuardianAgentRef       `json:"observedAiAgentVersion,omitempty"`
	ObservedBundle         *GuardianObservedBundle `json:"observedBundle,omitempty"`
	LastObservedOn         string                  `json:"lastObservedOn,omitempty"`
}

// GuardianObservedBundle is the rules file that ran in the agent's latest issue.
type GuardianObservedBundle struct {
	ID         string `json:"id"`
	ObservedOn string `json:"observedOn,omitempty"`
}

// GuardianCollectionOverview tells whether the agent's telemetry is collected.
type GuardianCollectionOverview struct {
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
	LastCollectedOn string `json:"lastCollectedOn,omitempty"`
}

// AgentRulesRunStarted is the accepted compilation run, without its id.
type AgentRulesRunStarted struct {
	State     string `json:"state"`
	CreatedOn string `json:"createdOn"`
}

// AgentRulesReport is the compilation report of the agent's latest run. The
// job id and each rule's guard detail are deliberately not decoded.
type AgentRulesReport struct {
	BundleID       string                 `json:"bundleId,omitempty"`
	AgentVersionID string                 `json:"agentVersionId"`
	StartedAt      string                 `json:"startedAt"`
	FinishedAt     string                 `json:"finishedAt"`
	Rules          []AgentRulesReportRule `json:"rules"`
}

// AgentRulesReportRule is one contract rule the run evaluated, and its outcome.
type AgentRulesReportRule struct {
	RuleID                 string `json:"ruleId"`
	AgentContractID        string `json:"agentContractId"`
	AgentContractVersionID string `json:"agentContractVersionId"`
	AgentContractLayer     string `json:"agentContractLayer"`
	Statement              string `json:"statement"`
	Outcome                string `json:"outcome"`
	Reason                 string `json:"reason,omitempty"`
}

// AgentRulesBundle is the latest rules file (enforcement bundle) of an agent.
type AgentRulesBundle struct {
	ID                 string `json:"id"`
	Version            int64  `json:"version"`
	AgentVersionID     string `json:"agentVersionId"`
	AgentFingerprintID string `json:"agentFingerprintId,omitempty"`
	SizeInBytes        int64  `json:"sizeInBytes"`
	ContentHash        string `json:"contentHash"`
	CompiledAt         string `json:"compiledAt"`
}

// GetAgentGuardianOverview reads one AI agent's state across Guardian.
func GetAgentGuardianOverview(ctx context.Context, collibraHttpClient *http.Client, aiAgentID string) (*AgentGuardianOverview, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading Guardian overview of AI Agent: %s", aiAgentID))

	var overview AgentGuardianOverview
	if err := getAgentScanJSON(ctx, collibraHttpClient, fmt.Sprintf(agentOverviewFmt, url.PathEscape(aiAgentID)), &overview); err != nil {
		return nil, err
	}
	return &overview, nil
}

// StartAgentRulesCompilation asks Guardian to compile the agent's rules file.
// Sent once on purpose: every request starts a run.
func StartAgentRulesCompilation(ctx context.Context, collibraHttpClient *http.Client, aiAgentID string) (*AgentRulesRunStarted, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Starting rules compilation for AI Agent: %s", aiAgentID))

	var run AgentRulesRunStarted
	if err := agentContractJSON(ctx, collibraHttpClient, http.MethodPost, fmt.Sprintf(agentCompileFmt, url.PathEscape(aiAgentID)), nil, &run); err != nil {
		return nil, err
	}
	if run.State == "" {
		return nil, errors.New("Collibra answered success but the response did not contain the run's state")
	}
	return &run, nil
}

// GetAgentRulesReport reads the compilation report of the agent's latest run.
func GetAgentRulesReport(ctx context.Context, collibraHttpClient *http.Client, aiAgentID string) (*AgentRulesReport, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading rules report of AI Agent: %s", aiAgentID))

	var report AgentRulesReport
	if err := getAgentScanJSON(ctx, collibraHttpClient, fmt.Sprintf(agentCompilationReportFmt, url.PathEscape(aiAgentID)), &report); err != nil {
		return nil, err
	}
	return &report, nil
}

// GetLatestAgentRulesBundle reads the metadata of the agent's latest rules file.
func GetLatestAgentRulesBundle(ctx context.Context, collibraHttpClient *http.Client, aiAgentID string) (*AgentRulesBundle, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading latest rules file of AI Agent: %s", aiAgentID))

	var bundle AgentRulesBundle
	if err := getAgentScanJSON(ctx, collibraHttpClient, fmt.Sprintf(agentLatestBundleFmt, url.PathEscape(aiAgentID)), &bundle); err != nil {
		return nil, err
	}
	if bundle.ID == "" {
		return nil, errors.New("Collibra answered success but the response did not contain a rules file id")
	}
	return &bundle, nil
}

// GetAgentRulesBundleDownloadURL asks for the rules file's download link and
// returns it from the redirect's Location header, without following it: the
// link is short-lived, points at object storage, and must not receive the
// Collibra credentials. The link is a secret, so it is never logged and never
// put in an error.
func GetAgentRulesBundleDownloadURL(ctx context.Context, collibraHttpClient *http.Client, bundleID string) (string, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Requesting download link of rules file: %s", bundleID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(bundleFileFmt, url.PathEscape(bundleID)), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	response, err := withoutRedirects(collibraHttpClient).Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	switch status := response.StatusCode; {
	case status >= 300 && status < 400:
		location := response.Header.Get("Location")
		if !isDownloadLink(location) {
			return "", signInRequiredError()
		}
		return location, nil
	case status >= 200 && status < 300:
		if looksLikeHTML(body) {
			return "", signInRequiredError()
		}
		return "", errors.New("Collibra answered without a download link for the rules file")
	default:
		return "", parseAgentScanAPIError(status, body)
	}
}

// withoutRedirects copies the client so a redirect is returned, not followed.
func withoutRedirects(c *http.Client) *http.Client {
	clone := *c
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clone
}

// isDownloadLink tells a signed object-storage link from a redirect to a
// sign-in page: the link is absolute, and a sign-in page is relative or names
// a sign-in path.
func isDownloadLink(location string) bool {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	path := strings.ToLower(parsed.Path)
	return !strings.Contains(path, "signin") && !strings.Contains(path, "login")
}

// GuardianFailure explains a failed Guardian Overview or Enforcement API call
// in plain language, with what to tell or ask the customer next.
func (e *AgentScanAPIError) GuardianFailure() (message, guidance string) {
	if text, ok := guardianErrorText[e.ErrorCode]; ok {
		message = text
	} else {
		switch e.StatusCode {
		case http.StatusUnauthorized:
			message = "Collibra did not accept the sign-in for this request (HTTP 401)."
		case http.StatusForbidden:
			message = "Collibra refused this request for the current user or environment (HTTP 403)."
		case http.StatusNotFound:
			message = "Collibra has nothing for this request (HTTP 404)."
		default:
			message = fmt.Sprintf("Collibra could not answer the request (HTTP %d).", e.StatusCode)
		}
	}
	if e.UserMessage != "" {
		message += " Collibra said: " + e.UserMessage
	}
	return message, guardianGuidance(e)
}

func guardianGuidance(e *AgentScanAPIError) string {
	switch {
	case e.ErrorCode == "aiAgentNotFound":
		return "Check that aiAgentId is the AI Agent asset id (list_scanned_agents returns it), not the agent's external id. If the agent was scanned a moment ago, its scan may still be importing: wait a minute and check again with get_agent_guardian_status."
	case e.ErrorCode == "noBundleAvailable":
		return "This is an answer, not a fault. Call get_agent_guardian_status to see why: no agent contract may govern the agent, the rules may never have been applied, a run may still be going, or the last run made no rules file because no rule could be enforced."
	case e.ErrorCode == "compilationReportNotFound":
		return "This is an answer, not a fault. Call get_agent_guardian_status: if the compilation is IN_PROGRESS, wait and ask again; if it is NOT_COMPILED, the rules were never applied for the agent's newest version."
	case e.ErrorCode == "bundleNotFound":
		return "A newer rules file may have replaced this one between the two reads. Ask once more; if it happens again, tell the customer and check get_agent_guardian_status."
	case e.ErrorCode == "agentContractsUnavailable":
		return "This is temporary. Wait a minute and try once more; if it keeps happening, tell the customer Collibra could not read the agent contracts just now."
	case e.StatusCode == http.StatusUnauthorized:
		return "Do not retry: signing in again is needed, not another attempt. Ask the customer to sign in to Collibra again (or check that their connection to this assistant is authorised), then start again."
	case e.StatusCode == http.StatusForbidden:
		return "Do not retry: the same sign-in will be refused again. Tell the customer they lack access to this agent or to Guardian in this Collibra environment, and to ask their Collibra administrator."
	case e.StatusCode == http.StatusBadRequest:
		return "Nothing was changed. Check that the id is a valid UUID and call again."
	}
	return "Tell the customer the request did not go through; trying once more later is fine, and if it keeps happening they can contact their Collibra administrator."
}

// guardianErrorText has one plain-language sentence per documented error code.
var guardianErrorText = map[string]string{
	"accessDenied":               "The user may not view this AI agent in Collibra (HTTP 403).",
	"aiAgentNotFound":            "Collibra's Guardian has no record of this AI agent: it was never scanned here, its scan is not imported yet, or the id is wrong.",
	"noBundleAvailable":          "No rules file has been made for this agent yet.",
	"compilationReportNotFound":  "There is no rules report for this agent yet: the rules were never applied for its newest version, or the report of the latest run is not imported yet.",
	"bundleNotFound":             "Collibra no longer knows this rules file.",
	"agentContractsUnavailable":  "Collibra could not read the agent contracts that govern this agent just now (HTTP 503).",
	"requestInvalid":             "Collibra rejected the request as malformed (HTTP 400).",
	AgentScanSignInRequiredCode:  "Collibra answered with a sign-in page, so the user is not signed in (or the session has expired).",
	"bearerTokenRequired":        "Collibra did not receive a valid sign-in token for this request (HTTP 401).",
	"authenticationFailed":       "Collibra did not accept the sign-in for this request (HTTP 401).",
	"tenantEnvironmentRequired":  "Collibra could not tell which environment this request is for (HTTP 401).",
	"tenantEnvironmentForbidden": "The sign-in is for a different Collibra environment than the one being read (HTTP 403).",
	"tenantEnvironmentDisabled":  "Guardian is switched off for this Collibra environment (HTTP 403).",
	"tenantEnvironmentNotFound":  "Guardian does not serve this Collibra environment (HTTP 403).",
}

// DescribeGuardianError turns an API error from the calls above into the
// pieces every Guardian setup tool reports. ok is false for an error that is
// not an API answer (a network problem), which each tool explains itself
// because a read and a write call for different advice.
func DescribeGuardianError(err error) (message, guidance, errorCode string, httpStatus int, ok bool) {
	var apiErr *AgentScanAPIError
	if !errors.As(err, &apiErr) {
		return "", "", "", 0, false
	}
	message, guidance = apiErr.GuardianFailure()
	return message, guidance, apiErr.ErrorCode, apiErr.StatusCode, true
}
