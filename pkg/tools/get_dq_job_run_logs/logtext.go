package get_dq_job_run_logs

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	maxLineLength = 400
	maxErrorLines = 30
)

// entry is one output line: a raw log line, or a run of consecutive lines that differ only by timestamp.
type entry struct {
	line      int // 1-based line number of the first raw line
	text      string
	count     int
	firstTime string
	lastTime  string
}

// render returns the entry as it is shown to the caller, already redacted and length-capped.
func (e entry) render() string {
	text := e.text
	if e.count > 1 {
		body := strings.Join(strings.Fields(timestampPattern.ReplaceAllString(text, "")), " ")
		switch {
		case e.firstTime != "":
			text = fmt.Sprintf("[repeated %sx, %s–%s] %s", commas(int64(e.count)), e.firstTime, e.lastTime, body)
		default:
			text = fmt.Sprintf("[repeated %sx] %s", commas(int64(e.count)), body)
		}
	}
	return truncateLine(redact(text))
}

// numbered prefixes the rendered entry with its line number so follow-up calls can page with offset.
func (e entry) numbered(marker string) string {
	return fmt.Sprintf("%d%s %s", e.line, marker, e.render())
}

// timestampPattern covers the formats seen in Edge logs: 2026-09-25 13:38:53, 2026/09/25 13:38:43,
// 26/09/25 13:40:58, and ISO forms with fractional seconds or a Z suffix.
var timestampPattern = regexp.MustCompile(`\b(?:\d{4}[-/]\d{2}[-/]\d{2}|\d{2}/\d{2}/\d{2})[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?Z?`)

// collapse folds consecutive lines that are identical once timestamps are removed. With on=false every
// raw line becomes its own entry.
func collapse(lines []string, on bool) []entry {
	entries := make([]entry, 0, len(lines))
	prevKey := ""
	for i, line := range lines {
		ts := clockOf(timestampPattern.FindString(line))
		key := timestampPattern.ReplaceAllString(line, "")
		if on && len(entries) > 0 && key == prevKey {
			last := &entries[len(entries)-1]
			last.count++
			if ts != "" {
				last.lastTime = ts
			}
			continue
		}
		entries = append(entries, entry{line: i + 1, text: line, count: 1, firstTime: ts, lastTime: ts})
		prevKey = key
	}
	return entries
}

func clockOf(ts string) string {
	if t, ok := parseTimestamp(ts); ok {
		return t.Format("15:04:05")
	}
	return ""
}

// parseTimestamp parses the first timestamp in s. Logs carry no zone, so the result is zone-less.
func parseTimestamp(s string) (time.Time, bool) {
	m := timestampPattern.FindString(s)
	if m == "" {
		return time.Time{}, false
	}
	m = strings.NewReplacer("/", "-", "T", " ", ",", ".").Replace(strings.TrimSuffix(m, "Z"))
	if i := strings.IndexByte(m, '.'); i > 0 {
		m = m[:i]
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "06-01-02 15:04:05"} {
		if t, err := time.Parse(layout, m); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ---- errors ----

// errorLinePattern matches error-looking lines. \w+Error\b catches JVM errors such as OutOfMemoryError.
var errorLinePattern = regexp.MustCompile(`(?i:\b(error|fatal)\b|exception|caused by:)|\w+Error\b`)

// verboseLevel skips DEBUG/TRACE lines (e.g. the OpenTelemetry agent), which mention exceptions routinely.
var verboseLevel = regexp.MustCompile(`\b(DEBUG|TRACE)\b`)

// collectErrors returns distinct error lines, in log order, each with its first line number and count.
func collectErrors(entries []entry) ([]string, bool) {
	type agg struct {
		first entry
		count int
	}
	var order []string
	seen := map[string]*agg{}
	for _, e := range entries {
		if verboseLevel.MatchString(e.text) || !errorLinePattern.MatchString(e.text) {
			continue
		}
		key := timestampPattern.ReplaceAllString(e.text, "")
		if a, ok := seen[key]; ok {
			a.count += e.count
			continue
		}
		seen[key] = &agg{first: e, count: e.count}
		order = append(order, key)
	}
	out := make([]string, 0, min(len(order), maxErrorLines))
	for _, key := range order[:min(len(order), maxErrorLines)] {
		a := seen[key]
		single := a.first
		single.count = 1
		line := single.numbered(":")
		if a.count > 1 {
			line = fmt.Sprintf("%s [%sx]", line, commas(int64(a.count)))
		}
		out = append(out, line)
	}
	return out, len(order) > maxErrorLines
}

// ---- raw sections ----

func headOf(entries []entry, n int) []string {
	return numberAll(entries[:min(n, len(entries))])
}

func tailOf(entries []entry, n int) []string {
	return numberAll(entries[max(len(entries)-n, 0):])
}

// rangeOf returns entries whose first raw line falls in (offset, offset+limit].
func rangeOf(entries []entry, offset, limit int) []string {
	var out []entry
	for _, e := range entries {
		if e.line > offset && e.line <= offset+limit {
			out = append(out, e)
		}
	}
	return numberAll(out)
}

// grepOf returns one block per match (merged when context overlaps). Matched lines are marked '>'.
func grepOf(entries []entry, re *regexp.Regexp, context, maxMatches int) (blocks []string, total int) {
	var matches []int
	for i, e := range entries {
		if re.MatchString(e.text) {
			total++
			if len(matches) < maxMatches {
				matches = append(matches, i)
			}
		}
	}
	matched := map[int]bool{}
	for _, i := range matches {
		matched[i] = true
	}
	for k := 0; k < len(matches); {
		start, end := max(matches[k]-context, 0), min(matches[k]+context, len(entries)-1)
		k++
		for k < len(matches) && matches[k]-context <= end+1 {
			end = min(matches[k]+context, len(entries)-1)
			k++
		}
		var b strings.Builder
		for i := start; i <= end; i++ {
			if i > start {
				b.WriteByte('\n')
			}
			marker := "|"
			if matched[i] {
				marker = ">"
			}
			b.WriteString(entries[i].numbered(marker))
		}
		blocks = append(blocks, b.String())
	}
	return blocks, total
}

func numberAll(entries []entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.numbered("|")
	}
	return out
}

// ---- helpers ----

func truncateLine(line string) string {
	if len(line) <= maxLineLength {
		return line
	}
	return line[:maxLineLength] + "…"
}

// evidence formats a raw log line as diagnostic evidence.
func evidence(stage string, lineNo int, text string) string {
	return fmt.Sprintf("%s L%d: %s", stage, lineNo, truncateLine(redact(strings.TrimSpace(text))))
}

func commas(n int64) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return "-" + commas(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func splitLines(text string) []string {
	return strings.Split(strings.TrimRight(strings.ReplaceAll(text, "\r\n", "\n"), "\n"), "\n")
}
