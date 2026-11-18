---
name: collibra-role-holders-to-group
description: |
  Find the people who hold a Collibra resource role (for example Product Owner or Business Analyst) and put
  them into a user group, one role to one group, with one approval, asked when the plan is complete and before anything is written. Use when the user asks
  to "put all Product Owners into a group", "make a user group from everyone with role X", "sync role holders
  to a group", or to list who holds a role across communities and domains. Needs the chip MCP server started
  with the access-management experimental feature. Reads run freely; the writes (creating groups, adding members) wait for
  one approval of the whole plan.
metadata:
  version: "0.1.0"
---

# Collibra: put the holders of a role into a user group

A **resource role** (Product Owner, Business Analyst, Steward, ...) is given to a user or a user group *on a
resource* (a community, domain or asset). That assignment is called a **responsibility**. A **user group** is a
named set of users that can be given roles together. This skill turns "everyone who holds role R" into "members of
group G".

## Approval rule (read this first)

There is **one approval**, asked once, at the moment the skill is ready to write.

- **Everything before the write runs without asking:** `search_roles`, `find_responsibilities`, `list_communities`,
  `list_domains`, `search_users`, `search_user_groups`. Say what you are about to look up in one short line, then
  do it. Do not ask permission for reads.
- **When the plan is complete, show it and ask once.** The plan names every write that will happen: which groups
  will be created, and which people will be added to which group. Ask a single question: "Approve this plan?".
- **One "yes" covers exactly that plan:** creating the missing groups and adding the listed people, in one go,
  with `confirm=true`. Do not ask again for each group or each batch.
- **Anything outside the plan needs a new approval:** another group, extra people, removing members
  (`remove_users_from_user_group`), renaming or deleting a group, creating or changing users. If the situation
  changes after the user approved (new people appear, a group name is taken), stop, show the change, and ask again.
- If a UI or another front end shows the plan and has its own approval button, that approval is the one approval.
- Never write before the user has approved the plan.

## Before you start

1. The chip tools must be present (`search_roles` among them). If not, tell the user the chip MCP server needs
   `--experimental=access-management` and a reconnect (`/mcp`), and stop.
2. Get from the user, asking only for what is missing:
   - the **role name(s)**, e.g. "Product Owner", "Business Analyst";
   - the **target group name** for each role, e.g. "Product Owners", "Business Analysts";
   - optionally a **scope**: the whole instance (default) or one community or domain.

## Steps

### 1. Find the role (read)
`search_roles(name=<role>, nameMatchMode=EXACT, type=RESOURCE)`.
- No result: stop and say the role does not exist. These tools cannot create roles. List near matches with
  `search_roles(name=<part of the name>)` so the user can pick one.
- Several results: show them and ask which one is meant.

### 2. Find who holds it (read)
`find_responsibilities(roleIds=[<role id>], limit=1000)`. Page with `offset` while `total` is larger than what you
have. With a scope, add `resourceIds=[<community or domain id>]` (get ids from `list_communities` /
`list_domains`; with a community, `includeInherited` decides whether inherited assignments count).

Each row has an `owner` with a type:
- **User:** collect the user id.
- **UserGroup:** do **not** expand silently. Note the group and ask the user later whether its members count.

Remove duplicates: one person can hold the role on many resources.

### 3. Resolve the people (read)
`search_users(userId=[...up to 50 per call...], includeDisabled=true, limit=50)`. Report people who are
**disabled** and leave them out unless the user wants them in. Report ids that match no user.

### 4. Show the plan (no write yet)
Present, per role:
- how many assignments, on how many resources, held by how many different people;
- the list of people (user name, first and last name);
- group owners found, disabled users, unknown ids;
- the target group and whether it already exists (`search_user_groups(name=<group>, nameMatchMode=EXACT)`), and if
  it does, who is in it (`search_users(groupId=<id>, includeDisabled=true)`).

Then ask which parts of the plan to carry out. Do not write yet.

### 5. Ask for the one approval
Show the plan from step 4 as the final list of writes, for example: "Create 2 groups; add 14 people to Product
Owners and 12 to Business Analysts." Ask "Approve this plan?". Do not write anything yet.

### 6. Write (after the approval, in one go)
For each role in the plan:
- If the group is missing: `create_user_group(name=<group>, confirm=true)`. If the tool answers `needs_input` because
  the name is taken, use the existing group it names.
- `add_users_to_user_group(userGroupId=<id>, userIds=[...], confirm=true)`. The tool skips people who are already
  members and reports unknown ids; large lists are fine, it looks users up in batches.

Do not call these tools with `confirm=false` first: the plan already was the preview. If a write answers
`needs_input` or `error`, stop and report it in plain words.

### 7. Check the result (read)
`search_users(groupId=<id>, includeDisabled=true, limit=1000)`. Report how many members the group has now and
compare with the plan. Mention any difference.

### 8. Several roles
With several roles, do steps 1-4 for all of them first, so that the single approval in step 5 covers every
group and every list of people, and then do steps 6-7 for each role.

## Keeping a group in line with the role (only if asked)

Adding people never removes anyone. If the user wants the group to match the role exactly, compare the group's
members with the role holders, show the extra members, and only then ask
for a separate approval of those removals. Removing people is outside the plan: never do it on your own.

## Things to know

- Tools return **no email addresses**, and `create_user` sets no password.
- A group's members get every role the group holds, so check what a group holds before adding people:
  `find_responsibilities(ownerIds=[<group id>])`.
- What a call may do is what the signed-in Collibra user may do. A `403` means they lack the permission: report it,
  do not retry.
- Every tool call answers with a `status`: `confirm_required` (preview, nothing written), `needs_input` (fix the
  input as the message says), `error` (read `message` and `guidance`). Report them in plain words.
- Defaults: searches return 20 rows unless `limit` is set (at most 1000). Always check `total`.

## Example

User: "Put all Product Owners into a group called Product Owners."
1. Find the role "Product Owner" and who holds it (reads), resolve the people (read).
2. Show: "Product Owner is held by 14 people on 31 resources; 2 are disabled; group 'Product Owners' does not
   exist. Plan: create the group and add 12 people." Ask once: "Approve this plan?".
3. After the yes: create the group and add the 12 people, without asking again.
4. Check the group: "12 members, as planned."
