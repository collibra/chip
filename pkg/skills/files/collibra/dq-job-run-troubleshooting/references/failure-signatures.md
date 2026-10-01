# Failure signatures → layer → fix

Use these as grep starting points with `dq_get_job_run_logs(run_id, grep=..., grep_context=5)`. They are case-insensitive via `(?i)`. A match is evidence, not a verdict — read the context lines and find the last `Caused by:`.

## Contents
1. Kubernetes / Spark resources (pullup)
2. Connection / network
3. Authentication / credentials
4. Authorization / permissions
5. Source query / schema
6. Source capacity / timeouts
7. Performance (ran but slow)
8. Dispatch / Edge site
9. Noise to ignore

---

## 1. Kubernetes / Spark resources (pullup)

| Signature | Regex | Meaning | Fix |
|---|---|---|---|
| Container killed for memory | `(?i)OOMKilled\|exit code:? ?137\|exitCode=137` | Pod exceeded its k8s memory **limit** (not the JVM heap) | Raise executor/driver memory *overhead* or limit in the Edge capability; or reduce data (columns, date window) |
| JVM heap OOM | `(?i)java\.lang\.OutOfMemoryError\|GC overhead limit exceeded\|Java heap space` | Heap too small for the partition/row volume | Reduce rows per partition (more partitions), narrow columns, then raise executor memory |
| Executor lost | `(?i)ExecutorLostFailure\|Lost executor\|executor .* exited` | Usually OOMKilled or node eviction; check the reason text | Follow the reason; if eviction, cluster capacity |
| Pods can't schedule | `(?i)Insufficient (cpu\|memory)\|FailedScheduling\|Pending` | Requests exceed free node capacity or namespace quota | Lower requests, scale the node pool, or raise quota (platform team) |
| Quota | `(?i)exceeded quota\|forbidden: .*quota` | Namespace ResourceQuota hit | Platform team raises quota, or run fewer concurrent jobs |
| Driver terminated | `(?i)SIGTERM\|exit code:? ?143` | Killed externally (timeout, eviction, cancellation) | Correlate with run CANCELLED / platform events |
| Image pull | `(?i)ImagePullBackOff\|ErrImagePull` | Edge can't pull Spark image (registry/proxy) | Edge admin: registry access, proxy config |

`sparkConfig` in the default response shows requests vs limits. If memory limit ≈ executor memory with no overhead headroom, OOMKilled is expected.

## 2. Connection / network

| Signature | Regex | Meaning | Fix |
|---|---|---|---|
| Refused | `(?i)Connection refused` | Nothing listening / firewall reject | Check host/port in the Edge connection; network path from Edge cluster |
| Timeout | `(?i)connect timed out\|SocketTimeoutException\|Read timed out` | Network path blocked or source overloaded | Allow-list Edge egress IPs; check proxy; raise JDBC socket timeout |
| DNS | `(?i)UnknownHostException\|Name or service not known` | Host not resolvable from Edge pods | Fix hostname or cluster DNS |
| TLS | `(?i)SSLHandshakeException\|PKIX path\|certificate` | Cert not trusted / TLS mismatch | Add CA to Edge truststore; fix `ssl` JDBC properties |
| Malformed URL / property | `(?i)MalformedURLException\|no protocol:` | A JDBC property expected a URL but got something else | Correct the named property in the Edge connection (e.g. Snowflake `authenticator` must be a valid value such as `snowflake`, `oauth`, `externalbrowser`, or an Okta URL) |
| Driver | `(?i)ClassNotFoundException\|No suitable driver` | JDBC driver missing / wrong version | Upload/select correct driver on the connection |

## 3. Authentication / credentials

| Signature | Regex | Fix |
|---|---|---|
| Bad login | `(?i)authentication failed\|invalid (username\|user\|password)\|login failed\|ORA-01017\|28P01` | Update credentials in the Edge connection / vault |
| Expired | `(?i)expired\|token.*invalid\|JWT` | Rotate key/token; check key-pair auth config |
| Vault | `(?i)vault\|secret.*not found` | Edge admin: vault path/credentials |
| Locked | `(?i)locked\|disabled` | Source DBA unlocks account |

Typical history pattern: long success streak, then fails every run from one date → credential rotation.

## 4. Authorization / permissions

| Signature | Regex | Fix |
|---|---|---|
| Generic | `(?i)permission denied\|insufficient privilege\|not authorized\|access denied` | Grant SELECT (and schema USAGE) to the connection's user/role |
| Snowflake | `(?i)does not exist or not authorized\|No active warehouse` | Grant USAGE on database/schema/warehouse; set default role/warehouse on the connection |
| Databricks / Unity Catalog | `(?i)PERMISSION_DENIED\|USE CATALOG\|USE SCHEMA` | Grant USE CATALOG, USE SCHEMA, SELECT |
| Postgres | `(?i)42501` | GRANT SELECT / USAGE |
| SQL Server | `(?i)SELECT permission was denied` | GRANT SELECT |

Pushdown jobs may also need privileges for temp objects or specific functions the generated SQL uses — if the denied object isn't the source table, quote it.

## 5. Source query / schema

| Signature | Regex | Fix |
|---|---|---|
| Missing column / table | `(?i)invalid identifier\|column .* (does not exist\|not found)\|Table .* (not found\|doesn't exist)\|ORA-00904\|ORA-00942\|42703\|42P01` | Source schema changed; update source query/columns via `dq_update_job` |
| Syntax | `(?i)syntax error\|SQL compilation error\|ParseException` | Fix source query; check dialect (quoting, backticks vs double quotes) |
| Type / cast | `(?i)cannot cast\|conversion failed\|invalid number\|NumberFormatException` | Cast explicitly in the source query |
| Run-date filter | Compare `executedQuery` `WHERE` clause with the job's run-date column | Wrong column/format → 0 rows or errors; fix run-date config |

Always read `executedQuery` from `dq_get_job_run` — it shows what actually ran.

## 6. Source capacity / timeouts

| Signature | Regex | Fix |
|---|---|---|
| Statement timeout | `(?i)statement timeout\|canceling statement due to\|query timeout\|STATEMENT_TIMEOUT_IN_SECONDS` | Narrow the query, or source DBA raises timeout |
| Warehouse | `(?i)warehouse .* suspended\|queued` | Resume/size warehouse; schedule outside peak |
| Locks | `(?i)lock wait\|deadlock` | Schedule away from ETL windows |

## 7. Performance (ran but slow)

Use `jobSummary` first: phase durations, slowest Spark stages, partitions, executors, rows loaded.

- **One phase dominates** — the JDBC read phase dominating means the source is slow or the pull isn't parallel. Spark compute dominating means sizing/skew.
- **Few partitions for many rows** (e.g. 1 partition, 20M rows) → single-threaded JDBC read. Recommend partitioned reads (partition column + bounds) if the capability exposes it, or pushdown.
- **Skew** — slowest task ≫ median in one stage → a hot key; consider filters or a different partition column.
- **Rows loaded jumped** vs last good run → data growth or a broken run-date filter (reading full table).
- **Executors idle / fewer than requested** → cluster capacity; see §1.
- `SELECT *` on wide tables → narrow the column list; biggest single win for both memory and time.

## 8. Dispatch / Edge site

- Stuck in WAITING/DISPATCHED with no logs → the Edge site isn't picking up work: site offline, capability misconfigured, or concurrency limit reached. Check other jobs on the same `edgeSiteName`.
- Many jobs on one site failing in the same window → site-level incident (upgrade, cluster outage). Don't tune individual jobs.
- Exception `Job interrupted` with no logs → run was stopped externally (cancel, Edge restart/upgrade, pod eviction). Check whether other runs were interrupted at the same time.

## 9. Noise to ignore

These show up in healthy runs; don't cite them as causes:
- `WARN NativeCodeLoader: Unable to load native-hadoop library`
- `WARN ... Utils: Service 'SparkUI' could not bind` (port retry)
- Log4j configuration warnings
- Single `retrying` lines followed by success
- Deprecation warnings
