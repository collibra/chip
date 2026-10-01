package get_dq_job_run_logs

import (
	"strings"
	"testing"
)

// Fake secrets only.
func TestRedactMasksSecrets(t *testing.T) {
	cases := []struct {
		name, in, secret string
	}{
		{"conf password", "spark.hadoop.fs.s3a.password=hunter2value", "hunter2value"},
		{"conf flag", "--conf spark.hadoop.fs.azure.account.key.acct=AbCdEf123456==", "AbCdEf123456=="},
		{"token colon", "access_token: s3cr3tT0k3n", "s3cr3tT0k3n"},
		{"credential", "gcpCredential=eyFAKEcredblob", "eyFAKEcredblob"},
		{"auth header", "Authorization: Bearer abcDEF123456789xyz", "abcDEF123456789xyz"},
		{"jdbc url key", "jdbcUrl=jdbc:postgresql://db:5432/prod?user=alice&password=pw", "alice"},
		{"jdbc userinfo", "connecting to jdbc:sqlserver://bob:pa55word@host:1433;db=x", "pa55word"},
		{"jdbc params", "url jdbc:snowflake://acct.snowflakecomputing.com/?user=carol;password=Zz9", "Zz9"},
		{"jdbc user param", "url jdbc:snowflake://acct.snowflakecomputing.com/?user=carol;warehouse=w", "carol"},
		{"spark tuple", "(spark.ssl.keyPassword,topSecretPw)", "topSecretPw"},
		{"aws key", "using key AKIAABCDEFGHIJKLMNOP for s3", "AKIAABCDEFGHIJKLMNOP"},
		{"gcp api key", "api AIzaSyA1234567890abcdefghijklmnopqrstuv ok", "AIzaSyA1234567890abcdefghijklmnopqrstuv"},
		{"azure sas", "https://acct.blob.core.windows.net/c?sv=2020&sig=AbC%2Bdef123", "AbC%2Bdef123"},
		{"jwt", "got eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.c2lnbmF0dXJlZmFrZQ here", "eyJzdWIiOiIxMjM0NSJ9"},
		{"private key", `"private_key": "-----BEGIN PRIVATE KEY-----MIIEvQIBADANfake-----END PRIVATE KEY-----"`, "MIIEvQIBADANfake"},
		{"url userinfo", "proxy https://dave:proxyPw1@proxy.local:8080", "proxyPw1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := redact(c.in)
			if strings.Contains(got, c.secret) {
				t.Errorf("secret %q leaked: %q", c.secret, got)
			}
		})
	}
}

func TestRedactKeepsOrdinaryLines(t *testing.T) {
	for _, line := range []string{
		"26/09/25 13:41:02 INFO Load: partitionNum sent from OwlOptions is : 1",
		"spark.executor.memory=1024M",
		"26/09/25 14:19:34 INFO DAGScheduler: ResultStage 0 (count at Load.scala:955) finished in 2306556 ms",
	} {
		if got := redact(line); got != line {
			t.Errorf("ordinary line changed:\n got %q\nwant %q", got, line)
		}
	}
}

func TestSparkConfigRedactsSensitiveConf(t *testing.T) {
	cfg := parseSparkConfig([]string{
		"2026-01-02 10:00:00 INFO  c.c.edge.launch.ClusterSparkLauncher - Spark settings:",
		"spark.executor.memory=4g",
		"spark.hadoop.fs.s3a.secret.key=FAKESECRETVALUE",
		"spark.jdbc.url=jdbc:postgresql://u:p@h/db",
		"2026-01-02 10:00:01 INFO  c.c.edge.spark.EdgeSparkSubmit - Got launcher",
		"2026-01-02 10:00:00 INFO  c.c.edge.spark.EdgeSparkSubmit - Edge parameters: EdgeParameters[sparkConf={spark.sql.password=FAKEPW2}]",
	})
	if cfg.Conf["spark.hadoop.fs.s3a.secret.key"] != redacted || cfg.Conf["spark.jdbc.url"] != redacted {
		t.Errorf("expected sensitive conf masked, got %v", cfg.Conf)
	}
	if cfg.Conf["spark.executor.memory"] != "4g" || *cfg.ExecutorMemory != "4g" {
		t.Errorf("expected ordinary conf kept, got %v", cfg.Conf)
	}
	if strings.Contains(*cfg.RawSubmitStatement, "FAKEPW2") {
		t.Errorf("rawSubmitStatement leaked a secret: %q", *cfg.RawSubmitStatement)
	}
}

func TestCollapseFoldsTimestampOnlyDifferences(t *testing.T) {
	lines := []string{
		"spark-submit: 26/09/25 13:41:10 INFO LoggingPodStatusWatcherImpl: Application status for spark-1 (phase: Running)",
		"spark-submit: 26/09/25 13:41:11 INFO LoggingPodStatusWatcherImpl: Application status for spark-1 (phase: Running)",
		"spark-submit: 26/09/25 13:41:12 INFO LoggingPodStatusWatcherImpl: Application status for spark-1 (phase: Running)",
		"spark-submit: 26/09/25 13:41:13 INFO LoggingPodStatusWatcherImpl: Application status for spark-1 (phase: Succeeded)",
	}
	entries := collapse(lines, true)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	want := "[repeated 3x, 13:41:10–13:41:12] spark-submit: INFO LoggingPodStatusWatcherImpl: Application status for spark-1 (phase: Running)"
	if got := entries[0].render(); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if entries[1].line != 4 {
		t.Errorf("expected the next entry to keep raw line number 4, got %d", entries[1].line)
	}
}
