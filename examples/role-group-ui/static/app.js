"use strict";

// Everything shown comes from Collibra or Claude, so it is only ever set as text, never as HTML.
const $ = (id) => document.getElementById(id);

function h(tag, props, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (key === "class") node.className = value;
    else if (key === "text") node.textContent = value;
    else node.setAttribute(key, value);
  }
  for (const child of children) if (child) node.append(child);
  return node;
}

const DEFAULT_MAPPINGS = [
  { role: "Product Owner", group: "Product Owners" },
  { role: "Business Analyst", group: "Business Analysts" },
];

let source = null;
let pending = null; // { planJobId, token, plan } while a plan waits for the one approval

// The plan belongs to the names it was made for, so the form is locked while a run or a plan is open.
function setSetupEnabled(enabled) {
  for (const control of $("setup").querySelectorAll("input, button")) control.disabled = !enabled;
}

function plural(count, one, many) {
  return count + " " + (count === 1 ? one : many);
}

function show(id, visible) {
  $(id).hidden = !visible;
}

function setError(id, message) {
  const node = $(id);
  node.textContent = message || "";
  node.hidden = !message;
}

function addMappingRow(role, group) {
  const row = h("div", { class: "mapping" });
  const roleInput = h("input", { type: "text", maxlength: "80", value: role || "", "data-field": "role" });
  const groupInput = h("input", { type: "text", maxlength: "80", value: group || "", "data-field": "group" });
  const remove = h("button", { type: "button", class: "secondary", text: "Remove" });
  remove.addEventListener("click", () => {
    if ($("mappings").children.length > 1) row.remove();
  });
  row.append(h("label", { text: "Role" }, roleInput), h("label", { text: "User group" }, groupInput), remove);
  $("mappings").append(row);
}

function readMappings() {
  return [...$("mappings").children].map((row) => ({
    role: row.querySelector('[data-field="role"]').value.trim(),
    group: row.querySelector('[data-field="group"]').value.trim(),
  }));
}

async function post(path, body) {
  const response = await fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(data.error || "The request failed (" + response.status + ")");
  return data;
}

function logLine(text) {
  $("log").append(h("li", { text: new Date().toLocaleTimeString() + "  " + text }));
}

function resetToSetup() {
  if (source) source.close();
  pending = null;
  for (const id of ["progress", "plan", "result", "approval", "fatal"]) show(id, false);
  show("setup", true);
  setSetupEnabled(true);
  $("log").replaceChildren();
}

function follow(jobId, handlers) {
  if (source) source.close();
  source = new EventSource("/api/jobs/" + jobId + "/events");
  const listen = (type, fn) => source.addEventListener(type, (event) => fn(JSON.parse(event.data)));
  listen("activity", (data) => {
    $("status").textContent = data.label + "…";
    logLine(data.label);
  });
  for (const [type, fn] of Object.entries(handlers)) listen(type, fn);
  listen("error", (data) => {
    show("progress", false);
    setError("fatal", data.message);
    show("fatal", true);
    show("setup", true);
    setSetupEnabled(true);
  });
  listen("end", () => source.close());
  source.onerror = () => source.close();
}

async function analyze() {
  setError("setup-error", "");
  setError("fatal", "");
  show("fatal", false);
  setSetupEnabled(false);
  try {
    const { jobId } = await post("/api/plan", { mappings: readMappings() });
    show("plan", false);
    show("result", false);
    show("approval", false);
    $("log").replaceChildren();
    $("status").textContent = "Starting…";
    show("progress", true);
    follow(jobId, { plan: renderPlan });
  } catch (error) {
    setError("setup-error", error.message);
    setSetupEnabled(true);
  }
}

function personTable(users, caption) {
  const table = h("table", {});
  table.append(h("caption", { class: "facts", text: caption }));
  table.append(h("thead", {}, h("tr", {}, h("th", { text: "User name" }), h("th", { text: "Name" }))));
  const body = h("tbody", {});
  for (const user of users) body.append(h("tr", {}, h("td", { text: user.userName }), h("td", { text: user.name || "" })));
  table.append(body);
  return table;
}

function stat(number, label) {
  return h("div", { class: "stat" }, h("div", { class: "n", text: String(number) }), h("div", { class: "l", text: label }));
}

function badge(text, kind) {
  return h("span", { class: "badge " + kind, text });
}

function renderPlan(data) {
  const plan = data.plan;
  pending = { planJobId: data.planJobId, token: data.token, plan };
  show("progress", false);
  const s = plan.stats;
  $("stats").replaceChildren(
    stat(s.communitiesTotal, "Communities in Collibra"),
    stat(s.communitiesWithAssignments, "Communities with these roles"),
    stat(s.assignments, "Role assignments found"),
    stat(s.usersFound, "Different users found"),
    stat(s.membershipsToAdd, "Users to add to groups"),
    stat(s.groupsToCreate, "Groups to create"),
  );
  $("warnings").replaceChildren(...plan.warnings.map((w) => h("div", { class: "warning", text: w })));
  $("plan-mappings").replaceChildren(...plan.mappings.map(mappingCard));
  show("plan", true);
  const writes = s.membershipsToAdd > 0 || s.groupsToCreate > 0;
  if (writes) {
    $("approval-summary").textContent =
      "Create " + plural(s.groupsToCreate, "group", "groups") + " and add " + plural(s.membershipsToAdd, "user", "users") +
      " to the groups shown above. Nothing else is changed.";
    $("approve").disabled = false;
    $("reject").disabled = false;
    show("approval", true);
  } else {
    setSetupEnabled(true);
  }
}

function mappingCard(m) {
  const head = h("div", { class: "mhead" });
  head.append(h("span", { class: "mtitle", text: m.role + "  →  " + m.group.name }));
  const badges = h("span", {});
  if (!m.roleFound) badges.append(badge("Role not found", "bad"));
  else badges.append(badge(m.group.exists ? "Group exists (" + m.group.currentMembers + " members)" : "Group will be created", m.group.exists ? "ok" : "warn"));
  head.append(badges);
  const body = h("div", { class: "mbody" });
  body.append(h("div", { class: "facts", text: m.assignments + " assignments on " + m.communities + " communities, held by " + m.holders.length + " enabled users" }));
  if (m.toAdd.length) body.append(personTable(m.toAdd, plural(m.toAdd.length, "user", "users") + " will be added to " + m.group.name));
  else if (m.roleFound) body.append(h("div", { class: "facts", text: "Nobody to add: everyone who holds the role is already in the group." }));
  if (m.alreadyMembers.length) {
    const details = h("details", {}, h("summary", { text: plural(m.alreadyMembers.length, "user is", "users are") + " already in the group (not added again)" }));
    details.append(personTable(m.alreadyMembers, "Already in " + m.group.name));
    body.append(details);
  }
  if (m.skipped.length) {
    const details = h("details", {}, h("summary", { text: m.skipped.length + " skipped" }));
    const list = h("ul", {});
    for (const item of m.skipped) list.append(h("li", { text: item.who + ": " + item.reason }));
    details.append(list);
    body.append(details);
  }
  return h("div", { class: "mcard" }, head, body);
}

async function approve() {
  if (!pending) return;
  $("approve").disabled = true;
  $("reject").disabled = true;
  try {
    const { jobId } = await post("/api/jobs/" + pending.planJobId + "/approve", { token: pending.token });
    show("approval", false);
    show("setup", false);
    $("log").replaceChildren();
    $("status").textContent = "Writing…";
    show("progress", true);
    pending = null;
    follow(jobId, { result: renderResult });
  } catch (error) {
    setError("fatal", error.message);
    show("fatal", true);
    $("reject").disabled = false;
  }
}

function renderResult(data) {
  show("progress", false);
  const result = data.result;
  $("result-summary").textContent = result.summary || "";
  const table = h("table", {});
  table.append(h("thead", {}, h("tr", {},
    h("th", { text: "Group" }), h("th", { text: "Created" }), h("th", { text: "Users added" }), h("th", { text: "Members now" }), h("th", { text: "Outcome" }))));
  const body = h("tbody", {});
  for (const m of result.mappings) {
    body.append(h("tr", {},
      h("td", { text: m.group }),
      h("td", { text: m.groupCreated ? "yes" : "no (existed)" }),
      h("td", { text: String(m.added) }),
      h("td", { text: String(m.membersAfter) }),
      h("td", {}, m.ok ? badge("done", "ok") : badge(m.error || "failed", "bad"))));
  }
  table.append(body);
  $("result-rows").replaceChildren(table);
  show("result", true);
}

$("add-mapping").addEventListener("click", () => {
  if ($("mappings").children.length < 5) addMappingRow("", "");
});
$("analyze").addEventListener("click", analyze);
$("approve").addEventListener("click", approve);
$("reject").addEventListener("click", resetToSetup);
$("restart").addEventListener("click", resetToSetup);
for (const m of DEFAULT_MAPPINGS) addMappingRow(m.role, m.group);
