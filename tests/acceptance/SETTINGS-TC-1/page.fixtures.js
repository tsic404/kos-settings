// The page projection GET /api/v1/pages/users answers, trimmed to the session's
// role, as fixtures for the rendered user page. The item list and source types
// are the ones SETTINGS-C-1 declares; the values are the shapes the product's
// read model returns for two applications and one volume.

export const OBSERVED_AT = "2026-03-01T12:00:00Z";
export const DATABASE_REASON = "控制面数据库不可用，成员与角色暂不可读";

const MEMBER_ENTRY = { method: "PUT", path: "/api/v1/accounts/{username}" };
const VISIBILITY_ENTRY = { method: "PUT", path: "/api/v1/resources/Application/{name}?namespace={namespace}" };
const OWNER_ENTRY = { method: "PUT", path: "/api/v1/resources/{kind}/{name}?namespace={namespace}" };

const members = [
  { username: "alice", role: "Owner" },
  { username: "bob", role: "Member" },
  { username: "carol", role: "Guest", disabled: true },
];

// visibilityRows is the visibility item's value for one Member: the named
// application owned by another account stays read-only, the owned one carries
// whatever the session may write.
function visibilityRows(shopWritable) {
  return [
    { namespace: "default", name: "shared", members: ["bob"], allowGuests: false, writable: false },
    { namespace: "default", name: "shop", members: ["bob", "carol"], allowGuests: false, writable: shopWritable },
  ];
}

const owners = [
  { kind: "Application", namespace: "default", name: "blog", owner: "alice" },
  { kind: "Application", namespace: "default", name: "shop", owner: "bob" },
  { kind: "Volume", namespace: "default", name: "data", owner: "alice" },
];

function item({ id, label, source, value, action, entry, reason = "" }) {
  return { id, label, source, value, observedAt: OBSERVED_AT, stale: false, action, reason, ...(entry ? { entry } : {}) };
}

// ownerPage is the projection an Owner reads: the three items with the writes
// KOSD-C-59 and KOSD-C-60 give that role, every visibility row writable.
export const ownerPage = {
  id: "users",
  title: "用户",
  location: "panel",
  readOnly: false,
  sources: ["database", "object"],
  items: [
    item({
      id: "users-members-and-roles", label: "成员与角色", source: "database",
      value: members, action: "set-member-role", entry: MEMBER_ENTRY,
    }),
    item({
      id: "users-app-visibility", label: "应用可见性", source: "object",
      value: [
        { namespace: "default", name: "blog", members: [], allowGuests: true, writable: true },
        { namespace: "default", name: "shop", members: ["bob", "carol"], allowGuests: false, writable: true },
      ],
      action: "set-app-visibility", entry: VISIBILITY_ENTRY,
    }),
    item({
      id: "users-app-and-volume-owners", label: "应用与卷所有者", source: "object",
      value: owners, action: "transfer-owner", entry: OWNER_ENTRY,
    }),
  ],
  actions: [],
};

// memberPage is the page of a Member that owns none of the applications it may
// see. The visibility action is declared for every role the page admits, so the
// item carries its entry; no row is writable, so the page offers no control
// (KOSD-C-59). The account write and the owner transfer stay the Owner's.
export const memberPage = {
  ...ownerPage,
  items: ownerPage.items.map((entry) =>
    entry.id === "users-app-visibility"
      ? item({
          id: entry.id, label: entry.label, source: entry.source, action: entry.action,
          value: visibilityRows(false), entry: VISIBILITY_ENTRY,
        })
      : item({ id: entry.id, label: entry.label, source: entry.source, value: entry.value, action: entry.action }),
  ),
};

// memberOwnerPage is the page of a Member owning shop: the visibility entry is
// offered on the rows it owns and nowhere else (KOSD-C-59).
export const memberOwnerPage = {
  ...memberPage,
  items: memberPage.items.map((entry) =>
    entry.id === "users-app-visibility"
      ? item({
          id: entry.id, label: entry.label, source: entry.source, action: entry.action,
          value: visibilityRows(true), entry: VISIBILITY_ENTRY,
        })
      : entry,
  ),
};

// guestPage is the projection a Guest reads: only the application open to
// guests, and no row it may write (KOSD-C-59).
export const guestPage = {
  ...memberPage,
  items: memberPage.items.map((entry) =>
    entry.id === "users-app-visibility"
      ? item({
          id: entry.id, label: entry.label, source: entry.source, action: entry.action,
          value: [{ namespace: "default", name: "blog", members: [], allowGuests: true, writable: false }],
          entry: VISIBILITY_ENTRY,
        })
      : entry,
  ),
};

// databaseDownPage is an Owner's page while the control-plane database does not
// answer: the member list states the reason and the object items stay readable
// (SETTINGS-C-1, KOSD-C-61).
export const databaseDownPage = {
  ...ownerPage,
  items: ownerPage.items.map((entry) =>
    entry.id === "users-members-and-roles"
      ? item({
          id: entry.id, label: entry.label, source: entry.source, action: entry.action, entry: entry.entry,
          value: { available: false, reason: DATABASE_REASON },
        })
      : entry,
  ),
};

// storedApplication is the document the control plane's read returns under
// `object` for shop: the write body has to keep it, minus the fields the
// server assigns (KOSD-C-03, KOSD-C-12).
export const storedApplication = {
  apiVersion: "kos.gokos.dev/v1beta1",
  kind: "Application",
  metadata: {
    name: "shop",
    namespace: "default",
    resourceVersion: "42",
    labels: { "kos.gokos.dev/managed-by": "kos" },
  },
  spec: {
    owner: "bob",
    namespace: "default",
    definitionRef: "shop",
    visibility: { members: ["bob", "carol"], allowGuests: false },
  },
  status: { health: { state: "Healthy" } },
};

// projectionOf wraps one stored document the way the control plane's read does.
export function projectionOf(object) {
  return { ref: { kind: object.kind, name: object.metadata.name }, observedAt: OBSERVED_AT, stale: false, object };
}
