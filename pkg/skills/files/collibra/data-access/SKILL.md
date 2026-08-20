---
description: Manages who can access what data, through grants, masks and filters.
related: collibra/discovery, collibra/index
---

# Data Access

Data Access is the system that manages who can access what data, through grants, masks and filters.

## Hard rules

1. Data Access users are not necessarily Collibra users. Data Access users are only also Collibra users if their email address is the same.
2. A group in Data Access is an access control with action Group. A Collibra group maps to the access control via its name.
3. Ownership of a data object does not mean that user has access to it.

## Search identities

`search_data_access_identities` is a tool to search for Data Access users (identities) by name and/or email. Providing `email` performs an exact lookup via `GetUserByEmail`. Providing `name` performs a server-side case-insensitive contains search via `SearchUsers`. Both can be combined: email resolves the user, name filters the result client-side. Name-only searches return up to `pageSize` matches (default 25, max 25); narrow the `name` filter to find results beyond that cap.

## Search Data Access objects

`search_data_access_objects` is a tool to search for data objects in Collibra Data Access (tables, columns, schemas, views, and other entities tracked in registered data sources). Filters can be combined: `name` (case-insensitive contains), `dataSources` (data source IDs), `types` (e.g. `table`, `column`, `schema`, `view`), `parents` / `ancestors` (other data object IDs to scope the search to a sub-tree), and `includeDeleted`. Returns up to `pageSize` matches (default 25, max 25). Each result includes the data object ID, name, fully qualified name, type, data type, deleted flag, description, data source ID, and `applicablePermissions` — the list of source-system permissions (each with a `name` and `description`) that can be requested on the object.

## Resolve a data source

`get_data_access_data_source` fetches a single data source by its **ID**, returning its `name`, `type` (e.g. `snowflake`, `databricks`, `bigquery`), `description`, `parentId`, and `createdAt` / `modifiedAt` timestamps. Use it to turn an opaque data source ID into a human-readable name. Data objects carry their data source as the `dataSourceId` field on results from `search_data_access_objects` and `check_user_data_object_access` — when the same object name exists in several data sources, fetch each `dataSourceId` to report which data source is which instead of printing raw IDs. If the ID does not resolve, the tool returns a `message` (and no `dataSource`); ask the user to correct it and call again.

## Check user access to data objects

`check_user_data_object_access` answers "Does a user have access to a data object (database, schema, table, view, column, etc.) and through which roles?". It takes one or more data object **IDs** in `objectIds` — not names. For every object it reports whether the user has access (`hasAccess`), the granted `permissions` and `globalPermissions`, and `roles` — the access controls that grant the access (each with `id`, `name`, `action`, `state`, and grant `category`). Required behavior:

- **WHO defaults to the current user.** Leave `userId` and `email` empty to check the calling user. To check a different user, resolve them first via `search_data_access_identities` and pass the returned ID in `userId` (or pass the user's `email`). Never pass a raw name.
- **WHAT must be resolved to IDs first.** Use `search_data_access_objects` to find the data object the user means, then pass its `id` in `objectIds`. When the name the user gave matches several objects (e.g. the same table in different schemas or data sources), present the candidates and let the user pick before checking access — do not guess.
- **Handle unresolved IDs.** IDs that do not correspond to an existing data object (`reason: not_found`) are returned in `result.unresolved`, and the tool sets a `message`. Ask the user to correct or drop those IDs, then call again. Objects that did resolve are still reported in `result.results`.
- **`expiresAt` caveat.** It is only populated when access is granted through a single access control; when multiple roles grant access it is `null`.

## Create an access request

`create_asset_access_request` is the only tool that creates a Collibra Data Access request. Destructive. Access is requested on a **catalog asset**, through the Data Access **role** linked to it. Required behavior:

- **`assetId` is any asset that has a role linked to it.** Which assets those are is instance configuration, and the tool reads the roles actually linked — never assume from the asset type. Resolve the asset the user named to a UUID first (`search_asset_keyword`, or the UUID after `/asset/` if they gave a URL).
- **Handle `no_role_linked`.** When the asset has no role that can be requested the tool returns that status (not an error), with the asset it rejected and `linkedRoles` — the access controls that are linked, if any. An empty `linkedRoles` means nothing is linked at all; a non-empty one means what is linked is not an active Grant (a mask, or a deactivated role), so say which. Ask the user for a different asset, or tell them an administrator has to link a role to this one. Never guess an alternative asset yourself.
- **A data product with no role of its own is expanded to its output ports** — on most instances access to a product is granted per port. When it has several the tool returns them in `outputPorts` with status `needs_port_selection`. Present them and let the user choose; then call again with that port's id in `outputPortId`. Never guess which port they mean. A product with exactly one output port is used directly. If the product itself carries a role, it is requested as-is — unless the user names a port in `outputPortId`, which always wins. `outputPortId` applies to nothing but a data product.
- **Never pass a role or a data object.** The WHAT is the role linked to the resolved asset, and the tool resolves it.
- **WHO is Collibra users and/or groups.** `users` takes email addresses (a Collibra username also works), mapped to Data Access users by email — the two systems only match on email (hard rule 1). `groups` takes group names (a Collibra group UUID also works), mapped to Data Access groups by name, which is the only identifier groups share. At least one beneficiary is required; both can be supplied together.
- **Nothing is created if a beneficiary does not map.** Whatever failed comes back in `unresolvedUsers` / `unresolvedGroups` with a reason — including a group name that matches several Data Access groups, which is reported rather than guessed. Ask the user to correct or drop those entries, then call again. `search_data_access_identities` helps find the address a user actually has in Data Access.
- **`expiresAt` is mandatory** — an access request cannot be open-ended. Ask the user when the access should end and pass a date (`2026-12-31`, taken as the end of that day UTC) or an RFC 3339 timestamp. Never invent an expiration date.
- **Purpose** is mandatory and must come from the user — it is the business justification for the request. If the user has not stated one, ask before calling the tool. Do not invent a purpose. The tool always appends a note stating the request was created by AI.
- **Name** is optional. If the user does not provide one, omit `name` on the first call. The tool returns status `needs_name_confirmation` with a `suggestedName` derived from the purpose — present it, get confirmation or an alternative, and call again with the confirmed value in `name`.
- **Report what was requested**, not just success: the asset, the role it was requested through, the mapped users, and the expiration date are all returned.

## Inspect an access control

`get_data_access_control_details` fetches a single Collibra Data Access control by its **ID**, returning its name, description, state (`ACTIVE`/`INACTIVE`/`DELETED`), action type (`GRANT`/`MASK`/`FILTER`/`SHARE`/`GROUP`/`FILTERRULE`), grant category, policy rule, external-management status, and the full `what`/`who` scope lists. Use this to inspect an individual access control once you have its ID — for example, from a `roles` entry returned by `check_user_data_object_access`.

## Common follow-ups

- Found a **data source id** → `get_data_access_data_source` to resolve its `name` and `type` (e.g. `snowflake`, `databricks`, `bigquery`).
