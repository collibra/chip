---
description: Manages who can access what data, through grants, masks and filters.
related: collibra/discovery, collibra/index
---

# Data Access

Data Access is the system that manages who can access what data, through grants, masks and filters.

## Hard rules

1. Data Access users are not necessarily DGC users. Data Access users are only also DGC users if their email address is the same.
2. Ownership of a data object does not mean that user has access to it.

## Search identities

`search_data_access_identities` is a tool to search for Data Access users (identities) by name and/or email. Providing `email` performs an exact lookup via `GetUserByEmail`. Providing `name` performs a server-side case-insensitive contains search via `SearchUsers`. Both can be combined: email resolves the user, name filters the result client-side. Name-only searches return up to `pageSize` matches (default 25, max 25); narrow the `name` filter to find results beyond that cap.

## Search Data Access objects

`search_data_access_objects` is a tool to search for data objects in Collibra Data Access (tables, columns, schemas, views, and other entities tracked in registered data sources). Filters can be combined: `name` (case-insensitive contains), `dataSources` (data source IDs), `types` (e.g. `table`, `column`, `schema`, `view`), `parents` / `ancestors` (other data object IDs to scope the search to a sub-tree), and `includeDeleted`. Returns up to `pageSize` matches (default 25, max 25). Each result includes the data object ID, name, fully qualified name, type, data type, deleted flag, description, data source ID, and `applicablePermissions` — the list of source-system permissions (each with a `name` and `description`) that can be requested on the object. Use those names when populating `what[].permissions` for `create_data_access_request`.

## Resolve a data source

`get_data_access_data_source` fetches a single data source by its **ID**, returning its `name`, `type` (e.g. `snowflake`, `databricks`, `bigquery`), `description`, `parentId`, and `createdAt` / `modifiedAt` timestamps. Use it to turn an opaque data source ID into a human-readable name. Data objects carry their data source as the `dataSourceId` field on results from `search_data_access_objects` and `check_user_data_object_access` — when the same object name exists in several data sources, fetch each `dataSourceId` to report which data source is which instead of printing raw IDs. If the ID does not resolve, the tool returns a `message` (and no `dataSource`); ask the user to correct it and call again.

## Check user access to data objects

`check_user_data_object_access` answers "Does a user have access to a data object (database, schema, table, view, column, etc.) and through which roles?". It takes one or more data object **IDs** in `objectIds` — not names. For every object it reports whether the user has access (`hasAccess`), the granted `permissions` and `globalPermissions`, and `roles` — the access controls that grant the access (each with `id`, `name`, `action`, `state`, and grant `category`). Required behavior:

- **WHO defaults to the current user.** Leave `userId` and `email` empty to check the calling user. To check a different user, resolve them first via `search_data_access_identities` and pass the returned ID in `userId` (or pass the user's `email`). Never pass a raw name.
- **WHAT must be resolved to IDs first.** Use `search_data_access_objects` to find the data object the user means, then pass its `id` in `objectIds`. When the name the user gave matches several objects (e.g. the same table in different schemas or data sources), present the candidates and let the user pick before checking access — do not guess.
- **Handle unresolved IDs.** IDs that do not correspond to an existing data object (`reason: not_found`) are returned in `result.unresolved`, and the tool sets a `message`. Ask the user to correct or drop those IDs, then call again. Objects that did resolve are still reported in `result.results`.
- **`expiresAt` caveat.** It is only populated when access is granted through a single access control; when multiple roles grant access it is `null`.

## Create Data Access request

`create_data_access_request` is a tool to create a new Collibra Data Access request on behalf of one or more users for one or more data objects. Destructive. Required behavior:

- **Minimum input is WHO, WHAT, and a purpose.** Do not call this tool until all three are supplied.
- **WHO** must be resolved via `search_data_access_identities` (by email or name) — pass the returned user IDs in `userIds`. Never pass raw emails or names.
- **WHAT** must be resolved via `search_data_access_objects` — pass the returned data object IDs in `what[].dataObjectId`. Per item, `permissions` should be empty and `globalPermissions` must always be READ.
- **Purpose** is mandatory and must come from the user — it is the business justification for the request. If the user has not stated a purpose, ask them for one before calling the tool. Do not invent a purpose. The tool always appends a note stating that the request was created by AI.
- **Name** is optional. If the user does not provide one, omit `name` on the first call. The tool will return status `needs_name_confirmation` with a `suggestedName` derived from the purpose — present that suggestion to the user, get their confirmation (or an alternative), and call again with the confirmed value in `name`.

## Inspect an access control

`get_data_access_control_details` fetches a single Collibra Data Access control by its **ID**, returning its name, description, state (`ACTIVE`/`INACTIVE`/`DELETED`), action type (`GRANT`/`MASK`/`FILTER`/`SHARE`/`GROUP`/`FILTERRULE`), grant category, policy rule, external-management status, and the full `what`/`who` scope lists. Use this to inspect an individual access control once you have its ID — for example, from a `roles` entry returned by `check_user_data_object_access`.

## Common follow-ups

- Found a **data source id** → `get_data_access_data_source` to resolve its `name` and `type` (e.g. `snowflake`, `databricks`, `bigquery`).
