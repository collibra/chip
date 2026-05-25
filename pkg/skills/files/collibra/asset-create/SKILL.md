---
description: Create new Collibra assets with create_asset, including RICH_TEXT attribute handling and duplicate gating.
related: collibra/asset-edit, collibra/discovery
shared: rich-text-markdown.md
---

# Creating assets

`create_asset` is a single-call write tool that resolves human-friendly identifiers
server-side: `assetType` accepts UUID, publicId (e.g. `"BusinessTerm"`), or display name
(e.g. `"Business Term"`); `domain` accepts UUID or name; `status` accepts UUID or status
name; `attributes` reference attribute types by `name` or `typeId`. You almost never need to
pre-resolve any of these.

## Hard rules

1. **`create_asset` is self-sufficient.** Do not call `prepare_create_asset` first by default.
   `create_asset` does its own resolution, validation, and duplicate check, and its error
   responses include suggestions for self-correction.
2. **Write Markdown in RICH_TEXT attributes.** Chip converts Markdown to HTML server-side for
   any RICH_TEXT attribute (e.g. `Definition`). Use `**bold**`, `[links](url)`, lists, and
   headings naturally. Never pass HTML; never pre-render Markdown yourself. See
   `shared/rich-text-markdown.md` for the full rules.
3. **Read the `status` field in the response.** Branch on `success`, `duplicate_found`,
   `validation_error`, or `error` — do not assume success.

## Workflow

1. Call `create_asset` with `name`, `assetType` (UUID, publicId, or display name), `domain`
   (UUID or name), optional `status`, and optional `attributes`.
2. Branch on the response `status`:
   - **`success`** — done. The response includes the new asset's UUID and a per-attribute
     outcome list. Report the UUID to the user.
   - **`duplicate_found`** — an asset with the same `name` already exists in the resolved
     (assetType, domain). The response includes the existing asset's ID. **Confirm with the
     user** before retrying with `allowDuplicate: true`. Do not auto-retry — duplicates are
     usually a mistake.
   - **`validation_error`** — the error message includes suggestions (available asset types,
     compatible domains, valid attribute names). Self-correct from the suggestions and
     retry. Only escalate to the user if a second attempt also fails.
   - **`error`** — an unexpected downstream Collibra failure. Surface the message to the
     user; do not retry blindly.

## When to call `prepare_create_asset` first

Optional companion tool. Only useful when:

- The user is browsing — "what asset types can I create?", "what domains accept a Business
  Term?".
- You need to inspect the full attribute schema before composing values (e.g. to know which
  attributes are RICH_TEXT or required). Pass `includeStringType=true` to see the
  attribute's stringType and description.
- A `create_asset` call returned `validation_error` and the message's suggestions were
  truncated. Use `prepare_create_asset` to enumerate the full option set, then retry.

For straightforward creates where the user gave you asset type + domain, skip
`prepare_create_asset` entirely.

## Attribute reference

- Reference attributes by `name` (e.g. `[{"name": "Definition", "value": "Monthly Recurring
  Revenue"}]`) or by `typeId`. Names are case-sensitive against Collibra's attribute type
  display name.
- For RICH_TEXT attributes, the `value` is treated as Markdown. See
  `shared/rich-text-markdown.md`.
- An unknown attribute name returns `validation_error` with a list of valid names — self-
  correct using that list.

## Edits, not creates

To modify an existing asset, use `edit_asset` — see `collibra/asset-edit`. `create_asset`
with `allowDuplicate: true` always creates a new asset; it never merges or updates.
