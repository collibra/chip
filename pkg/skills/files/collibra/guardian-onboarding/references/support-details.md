# Support details

Load this only when you stop on a failure you can't explain or fix from here, after the four-part failure message in `references/verify-and-diagnose.md`.

Show the block below, in plain labels, with every id this run has. Leave out a line you have no value for; never invent one (H3). Times are in UTC.

```text
If you contact Collibra support, share these details:
Instance:       <instance URL>
When (UTC):     <start> to <end>
Step:           <n>, <the step's name as the customer saw it>
What happened:  <plain words>
Agent:          <AI Agent id>
Agent version:  <AI Agent Version id>
Scan:           <scan id>

Rules file:     <rules file id>
Use case:       <AI Use Case id>
Contracts:      <agent contract ids>
```

Never include:

- a job id, or anything else that describes how Collibra works inside;
- a download link, a token, a secret, or anything from the agent's code or prompts;
- internal system names, queries, log searches or links to internal tools;
- this skill's labels or stage numbers (use the customer's step number and name).

Then: "Nothing else was changed." if that is true, or a plain list of what was changed before the failure.
