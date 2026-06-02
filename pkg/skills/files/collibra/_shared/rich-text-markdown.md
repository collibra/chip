# RICH_TEXT attributes and Markdown

Some Collibra attribute types are `RICH_TEXT` — the Collibra UI renders them as formatted
HTML. The canonical example is `Definition` on Business Term assets.

Chip detects RICH_TEXT attributes server-side and **converts Markdown to HTML before
writing**, so you can author the value as natural Markdown and it will render correctly in
the Collibra UI.

## Rules

1. **Always write Markdown for RICH_TEXT attributes.** Use the standard syntax: `**bold**`,
   `*italic*`, `[text](url)`, bullet lists (`-`), numbered lists, headings (`#`, `##`),
   inline `code`, fenced code blocks, blockquotes, tables.
2. **Never write HTML.** Passing HTML directly bypasses the conversion and produces escaped
   tags in the rendered output.
3. **Never pre-render Markdown yourself.** Pass the raw Markdown source. Chip's
   `pkg/markdown` package handles the conversion via Goldmark.
4. **Plain-text attribute types pass through unchanged.** Markdown syntax in a non-RICH_TEXT
   attribute will be stored literally — no conversion happens.

## Identifying RICH_TEXT attributes

If unsure whether an attribute is RICH_TEXT:

- Call `prepare_create_asset` with both `assetType` and `domain` plus `includeStringType=true`.
- In the response, each attribute carries a `stringType` field. `RICH_TEXT` is the marker.
- Other common types: `PLAIN_TEXT` (single-line), `SCRIPT` (treated as code).

## Examples

A Business Term's `Definition`:

```markdown
**Monthly Recurring Revenue (MRR)** is the predictable revenue a business expects to receive
each month from subscriptions.

It is calculated as:

- Active subscriptions × average plan price
- Excluding one-time fees and prorations

See also: [Annual Recurring Revenue](https://example.com/arr).
```

This renders in the Collibra UI as bold, a bulleted list, and a clickable link.
