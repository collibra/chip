package get_dq_job_run_logs

import (
	"regexp"
	"strings"
)

const redacted = "***"

// sensitiveKey matches config/parameter names whose values must never be returned.
var sensitiveKey = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|key|credential|auth|jdbc[\w.\-]*url)`)

var (
	// key=value, key: value, --conf key=value. The key is the token right before the separator; values
	// stop at brackets and '=' so pairs nested in "sparkConf={k=v}" are still visited.
	kvPattern = regexp.MustCompile(`([\w.\-]+)(\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;&=\[\](){}]+)`)
	// Spark's "(key,value)" tuples.
	tuplePattern     = regexp.MustCompile(`\(([\w.\-]+),([^)]*)\)`)
	jdbcUserInfo     = regexp.MustCompile(`(?i)(jdbc:[\w:]+//)[^/@\s:]+:[^/@\s]+@`)
	jdbcUserParam    = regexp.MustCompile(`(?i)([;?&](?:user|username|uid|user id)=)[^;&\s]+`)
	bearerToken      = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=\-]{8,}`)
	jwtPattern       = regexp.MustCompile(`\beyJ[\w-]{5,}\.[\w-]{5,}\.[\w-]{5,}`)
	awsAccessKey     = regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)
	gcpAPIKey        = regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`)
	azureSASSig      = regexp.MustCompile(`(?i)([?&;]sig=)[^&\s]+`)
	privateKeyBlock  = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(-----END [A-Z ]*PRIVATE KEY-----|$)`)
	urlUserInfoInURL = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.\-]*://)[^/@\s:]+:[^/@\s]+@`)
)

// redact masks secrets in one line of log text. It is applied to every log-derived string before it
// leaves the tool.
func redact(s string) string {
	s = privateKeyBlock.ReplaceAllString(s, "-----PRIVATE KEY "+redacted+"-----")
	s = jdbcUserInfo.ReplaceAllString(s, "${1}"+redacted+":"+redacted+"@")
	s = urlUserInfoInURL.ReplaceAllString(s, "${1}"+redacted+":"+redacted+"@")
	s = jdbcUserParam.ReplaceAllString(s, "${1}"+redacted)
	s = azureSASSig.ReplaceAllString(s, "${1}"+redacted)
	s = bearerToken.ReplaceAllString(s, "${1} "+redacted)
	s = jwtPattern.ReplaceAllString(s, redacted)
	s = awsAccessKey.ReplaceAllString(s, redacted)
	s = gcpAPIKey.ReplaceAllString(s, redacted)
	s = tuplePattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := tuplePattern.FindStringSubmatch(m)
		if sensitiveKey.MatchString(parts[1]) {
			return "(" + parts[1] + "," + redacted + ")"
		}
		return m
	})
	s = kvPattern.ReplaceAllStringFunc(s, func(m string) string {
		parts := kvPattern.FindStringSubmatch(m)
		if sensitiveKey.MatchString(parts[1]) && parts[3] != redacted {
			return parts[1] + parts[2] + redacted
		}
		return m
	})
	return s
}

// redactValue masks a config value whose key is sensitive, and otherwise scrubs it like any log text.
func redactValue(key, value string) string {
	if sensitiveKey.MatchString(key) && strings.TrimSpace(value) != "" {
		return redacted
	}
	return redact(value)
}
