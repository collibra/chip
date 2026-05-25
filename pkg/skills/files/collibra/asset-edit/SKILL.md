---
description: Modify existing Collibra assets via typed operations (attributes, properties, relations, tags, responsibilities).
related: collibra/asset-create, collibra/discovery
shared: rich-text-markdown.md
---

# Editing assets

`edit_asset` applies a list of **typed operations** to a single asset, identified by UUID.
Each operation has its own required fields; mixing operation types in one call is fine and
runs them in order.

## Operation types

| Operation | Use for | Key fields |
|---|---|---|
| `update_attribute` | Change an existing attribute value (e.g. update `Definition`) | `attributeName`, `value` |
| `add_attribute` | Append a new value to an attribute (multi-valued attributes only) | `attributeName`, `value` |
| `remove_attribute` | Clear an attribute value | `attributeName` (optionally a specific value) |
| `update_property` | Change asset-level properties: `name`, `displayName`, `statusId` | `field`, `value` |
| `add_relation` | Link this asset to another by relation role (e.g. `is synonym of`) | `relationType`, target asset identifier |
| `remove_relation` | Unlink a relation | `relationType`, target asset identifier |
| `add_tag` | Append a free-text tag (does not replace existing tags) | `tag` |
| `set_responsibility` | Assign a user or group to a resource role (e.g. `Steward`, `Owner`) | `role`, `userId` (UUID, username, or email) |

## Hard rules

1. **`update_attribute` vs `add_attribute`.** `update_attribute` fails if the attribute does
   not already exist on the asset — the error suggests calling `add_attribute` instead.
   `add_attribute` always appends and is valid only for multi-valued attribute types.
2. **`update_property` is restricted.** Only three fields are allowed: `name`, `displayName`,
   `statusId`. Other fields return an error listing the allowed set.
3. **`statusId` accepts names.** Pass a human-readable status name (e.g. `"Candidate"`,
   `"Accepted"`) or the UUID — chip resolves either.
4. **`set_responsibility` accepts user identifiers in three forms.** `userId` may be a UUID,
   a username, or an email. Chip resolves the form server-side. The same applies to
   `relationType` targets in `add_relation` / `remove_relation`.
5. **RICH_TEXT attribute values are Markdown.** Same rule as `create_asset` — write Markdown,
   chip converts to HTML. See `shared/rich-text-markdown.md` for the full rules.

## Workflow

1. Resolve the asset's UUID — usually via `search_asset_keyword` or one of the discovery
   tools (`collibra/discovery`).
2. Compose the list of operations. Each is a typed object with the fields above.
3. Call `edit_asset` once with the full list. Mixed operation types are allowed.
4. The response includes a per-operation result. Check each one — partial failures are
   possible. Operations that fail return an error message and do not roll back successful
   operations earlier in the list.

## When not to use `edit_asset`

- Creating a brand-new asset → `create_asset` (see `collibra/asset-create`).
- Adding a classification → `add_data_classification_match` (different permission scope and
  different ID space).
- Pushing a data contract manifest → `push_data_contract_manifest`.
