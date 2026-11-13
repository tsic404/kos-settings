// The write bodies the user page submits. The browser reads the object one
// entry writes through the control plane, changes the fields the form owns and
// submits the stored document back, so a save keeps every field it does not
// change and carries the version it read (KOSD-C-12, KOSD-C-59, KOSD-C-60).
// The functions are pure: the submission is judged without a browser.

// MEMBERS_SEPARATOR is the line break the member field joins with, and the
// field splits on that separator alone. A login name cannot carry a line break
// (the control plane rejects control characters), so a name holding 、, spaces
// or commas round-trips unchanged (KOSD-C-59).
const MEMBERS_SEPARATOR = "\n";
const MEMBERS_SPLIT = /\r?\n/;

// splitMembers reads the member field back as the accounts it names.
export function splitMembers(text) {
  return String(text || "")
    .split(MEMBERS_SPLIT)
    .map((entry) => entry.trim())
    .filter(Boolean);
}

// joinMembers fills the member field from a row's member list.
export function joinMembers(list) {
  const names = Array.isArray(list) ? list : [];
  return names
    .map((name) => String(name === null || name === undefined ? "" : name).trim())
    .filter(Boolean)
    .join(MEMBERS_SEPARATOR);
}

// storedDocument returns the object the control plane's read answered with: the
// projection wraps it under `object`, and only that is a writable document.
function storedDocument(projection, name) {
  const stored = projection && typeof projection.object === "object" && projection.object !== null ? projection.object : null;
  if (!stored) throw { code: "internal", message: "读取对象失败，未提交写入" };
  const metadata = { ...(stored.metadata || {}) };
  if (!metadata.name) metadata.name = name;
  const body = { metadata, spec: { ...(stored.spec || {}) } };
  if (stored.apiVersion) body.apiVersion = stored.apiVersion;
  if (stored.kind) body.kind = stored.kind;
  return body;
}

// visibilityBody is the write that changes one application's visibility. The
// object's other spec fields, labels and version stay as read, and status is
// left out because the reconciler owns it (KOSD-C-03).
export function visibilityBody(projection, name, membersText, allowGuests) {
  const body = storedDocument(projection, name);
  body.spec = { ...body.spec, visibility: { members: splitMembers(membersText), allowGuests } };
  return body;
}

// ownerBody is the write that transfers one object's owner.
export function ownerBody(projection, name, owner) {
  const body = storedDocument(projection, name);
  body.spec = { ...body.spec, owner: String(owner || "").trim() };
  return body;
}
