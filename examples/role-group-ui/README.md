# Example: role holders to user group (UI)

An example front end for the access-management tools (`--experimental=access-management`). A local page that runs the `collibra-role-holders-to-group` skill through Claude Code. You choose roles and the
user groups they should feed. Claude analyses, the page shows the plan, and **one approval** writes it.

```
python3 server.py        # then open http://127.0.0.1:8765
```

Needs:
- the `claude` CLI on `PATH`;
- the `chip` MCP server registered in Claude Code and started with `--experimental=access-management`
  (it reads its Collibra connection from `~/.config/collibra/mcp.yaml`);
- the skill installed: copy `skill/SKILL.md` to `~/.claude/skills/collibra-role-holders-to-group/SKILL.md`.
  It is deliberately not part of chip's embedded skill catalog: chip's skill gate only knows capability flags, not
  experimental features, so an embedded skill naming these tools would be served even when they are off.
- Python 3, standard library only.

## What you see
- **Statistics:** communities in Collibra, communities with the roles, role assignments, different users found,
  users to add, groups to create. The server works out the totals from the lists; it does not take Claude's sums.
- **Per role:** the group (new or existing), the users that will be added, who is already a member, and skipped
  users (disabled, not found).
- **One approval bar:** "Approve and write" or "Reject".
- **Result:** per group, whether it was created, how many users were added and how many members it has now.

## How it is kept safe
1. **Plan run:** read-only. The write tools are denied on the command line, so nothing can be written in this run.
2. **Approval:** the server keeps the plan itself and gives the page a one-time token. The page only sends the token;
   the plan that is written is the one the server stored. A plan can be approved once.
3. **Write run:** only `create_user_group` and `add_users_to_user_group` are allowed. Removing, renaming, deleting
   and creating or changing users stay denied.
4. One run at a time. The server listens on `127.0.0.1` only and checks the `Host` and `Origin` headers.

A run takes about two minutes and uses Claude tokens.

## Tests
```
python3 -m unittest tests.test_server      # stub in place of Claude, nothing touches Collibra
python3 tests/serve_with_stub.py           # the page on :8766 with the stub, for browser checks
```
