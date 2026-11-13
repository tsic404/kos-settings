// Package settingstc1 verifies the user page SETTINGS-C-1 puts in the panel:
// the three items with the source types the contract gives them, the
// modification entries KOSD-C-59 and KOSD-C-60 allow, the reason the member
// list states while the control-plane database is down, and the withdrawal of
// the page with its component (SETTINGS-AC-1, SETTINGS-TC-1).
package settingstc1

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/pages"
	"github.com/tsic404/kosd/pkg/write"

	"github.com/tsic404/kos-settings/pkg/kossettings"
	"github.com/tsic404/kos-settings/tests/acceptance/fixtures"
)

// userPageTitle is what the contract calls the page; the product chooses its
// identity, so the case finds it the way the panel's page list does.
const userPageTitle = "用户"

// userPage returns the user page the session is offered and renders it.
func userPage(t *testing.T, shell *fixtures.Shell, session identity.Session) pages.Page {
	t.Helper()
	for _, summary := range shell.Pages.List(session) {
		if summary.Title == userPageTitle {
			return fixtures.ReadPage(t, shell, session, summary.ID)
		}
	}
	t.Fatalf("%q is offered no user page: %+v", session.Username, shell.Pages.List(session))
	return pages.Page{}
}

// itemLabel returns one rendered item by the label the contract gives it.
func itemLabel(t *testing.T, page pages.Page, label string) pages.Item {
	t.Helper()
	for _, item := range page.Items {
		if item.Label == label {
			return item
		}
	}
	t.Fatalf("page %q has no item labelled %q: %+v", page.ID, label, page.Items)
	return pages.Item{}
}

// projectedValue returns one item's value as the browser receives it.
func projectedValue(t *testing.T, page pages.Page, label string) any {
	t.Helper()
	projected := fixtures.JSONOf(t, page)
	raw, ok := projected["items"].([]any)
	if !ok {
		t.Fatalf("page %q carries no items: %v", page.ID, projected)
	}
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("page %q carries a non-object item: %v", page.ID, entry)
		}
		if item["label"] == label {
			return item["value"]
		}
	}
	t.Fatalf("page %q has no item labelled %q: %v", page.ID, label, raw)
	return nil
}

// rowsOf reads one item's value as the list of rows the page shows.
func rowsOf(t *testing.T, page pages.Page, label string) []map[string]any {
	t.Helper()
	raw, ok := projectedValue(t, page, label).([]any)
	if !ok {
		t.Fatalf("item %q value is not a row list: %v", label, projectedValue(t, page, label))
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		row, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("item %q carries a non-object row: %v", label, entry)
		}
		rows = append(rows, row)
	}
	return rows
}

// rowMatching returns the row whose field equals want.
func rowMatching(t *testing.T, rows []map[string]any, field, want string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if row[field] == want {
			return row
		}
	}
	t.Fatalf("no row has %s = %q: %v", field, want, rows)
	return nil
}

// namesOf lists the name column of one item's rows in the order the page shows.
func namesOf(rows []map[string]any) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		name, _ := row["name"].(string)
		names = append(names, name)
	}
	return names
}

func TestTC1ThreeItemsFromTheirSources(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Application(t, cluster, "default", "shop", "owner", []string{"member"}, false)
	fixtures.Volume(t, cluster, "default", "data", "owner")
	roster := fixtures.NewRoster(
		identity.Account{Username: "owner", Role: write.RoleOwner},
		identity.Account{Username: "member", Role: write.RoleMember},
		identity.Account{Username: "guest", Role: write.RoleGuest, Disabled: true},
	)
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues(), roster)

	// SETTINGS-C-1 fixes the three items and one source type each: the member
	// list comes from the control-plane database, visibility and ownership from
	// the objects.
	sources := map[string]string{
		"成员与角色":   "database",
		"应用可见性":   "object",
		"应用与卷所有者": "object",
	}
	for _, session := range []identity.Session{fixtures.Owner, fixtures.Member, fixtures.Guest} {
		page := userPage(t, shell, session)
		if len(page.Items) != len(sources) {
			t.Fatalf("%q reads %d items, want the %d SETTINGS-C-1 declares: %+v",
				session.Username, len(page.Items), len(sources), page.Items)
		}
		for _, item := range page.Items {
			source, declared := sources[item.Label]
			if !declared {
				t.Fatalf("%q reads item %q, which SETTINGS-C-1 does not declare", session.Username, item.Label)
			}
			if item.Source != source {
				t.Fatalf("item %q source = %q, want %q (SETTINGS-C-1, KOSD-C-53)", item.Label, item.Source, source)
			}
			if value := projectedValue(t, page, item.Label); value == nil {
				t.Fatalf("item %q carries no current value", item.Label)
			}
		}
	}

	// The values are the sources the contract names: the roster with its roles,
	// the applications' visibility and the owned applications and volumes.
	page := userPage(t, shell, fixtures.Owner)
	members := rowsOf(t, page, "成员与角色")
	if len(members) != 3 {
		t.Fatalf("member list = %v, want the three accounts the database holds", members)
	}
	if role := rowMatching(t, members, "username", "member")["role"]; role != "Member" {
		t.Fatalf("member role = %v, want Member", role)
	}
	if disabled := rowMatching(t, members, "username", "guest")["disabled"]; disabled != true {
		t.Fatalf("guest disabled = %v, want the stored state", disabled)
	}
	visibility := rowsOf(t, page, "应用可见性")
	shop := rowMatching(t, visibility, "name", "shop")
	if shop["namespace"] != "default" || shop["allowGuests"] != false {
		t.Fatalf("visibility row = %v, want default/shop with guests refused", shop)
	}
	if listed, _ := shop["members"].([]any); len(listed) != 1 || listed[0] != "member" {
		t.Fatalf("visibility members = %v, want the listed member", shop["members"])
	}
	owners := rowsOf(t, page, "应用与卷所有者")
	if owner := rowMatching(t, owners, "name", "shop")["owner"]; owner != "owner" {
		t.Fatalf("application owner = %v, want the object's owner", owner)
	}
	if kind := rowMatching(t, owners, "name", "data")["kind"]; kind != "Volume" {
		t.Fatalf("volume row kind = %v, want Volume", kind)
	}
}

func TestTC1ModificationEntriesFollowTheRole(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Application(t, cluster, "default", "shop", "owner", nil, false)
	fixtures.Volume(t, cluster, "default", "data", "owner")
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues())

	// The Owner writes every item (KOSD-C-59): each carries the write that
	// changes it and no read-only reason, and every page action is bound to its
	// item rather than left for the page to guess.
	owner := userPage(t, shell, fixtures.Owner)
	actions := map[string]bool{}
	for _, action := range owner.Actions {
		actions[action.ID] = true
	}
	for _, item := range owner.Items {
		if item.Entry == nil {
			t.Fatalf("the owner sees no modification entry for %q", item.Label)
		}
		if item.Reason != "" {
			t.Fatalf("modifiable item %q carries the reason %q", item.Label, item.Reason)
		}
		if item.Entry.Method != "PUT" || !strings.HasPrefix(item.Entry.Path, "/api/v1/") {
			t.Fatalf("item %q entry = %+v, want a control-plane write", item.Label, item.Entry)
		}
		if item.Action == "" || !actions[item.Action] {
			t.Fatalf("item %q entry %q is not among the page's actions %+v", item.Label, item.Action, owner.Actions)
		}
		delete(actions, item.Action)
	}
	if len(actions) != 0 {
		t.Fatalf("the page offers unbound actions %+v, want every action on its item", owner.Actions)
	}
	// Each entry addresses the source its item declares and names the identity
	// the browser fills from the row it edits.
	if path := itemLabel(t, owner, "成员与角色").Entry.Path; path != "/api/v1/accounts/{username}" {
		t.Fatalf("role entry = %q, want the account write", path)
	}
	if path := itemLabel(t, owner, "应用可见性").Entry.Path; path != "/api/v1/resources/Application/{name}?namespace={namespace}" {
		t.Fatalf("visibility entry = %q, want the application write", path)
	}
	if path := itemLabel(t, owner, "应用与卷所有者").Entry.Path; path != "/api/v1/resources/{kind}/{name}?namespace={namespace}" {
		t.Fatalf("owner entry = %q, want the owned-object write", path)
	}

	// A Member and a Guest write no shown row on this page: account writes and
	// owner transfer are the Owner's, and the visibility rows carry no writable
	// row for them, so the page shows no control (KOSD-C-59).
	for _, session := range []identity.Session{fixtures.Member, fixtures.Guest} {
		page := userPage(t, shell, session)
		for _, label := range []string{"成员与角色", "应用与卷所有者"} {
			if entry := itemLabel(t, page, label).Entry; entry != nil {
				t.Fatalf("role %q receives the entry %+v for %q, want none (KOSD-C-59)", session.Role, entry, label)
			}
		}
		for _, row := range rowsOf(t, page, "应用可见性") {
			if row["writable"] != false {
				t.Fatalf("role %q may write the row %v, want none (KOSD-C-59)", session.Role, row)
			}
		}
	}
}

func TestTC1MemberWritesTheApplicationsItOwns(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Application(t, cluster, "default", "mine", "member", nil, false)
	fixtures.Application(t, cluster, "default", "shared", "owner", []string{"member"}, false)
	fixtures.Application(t, cluster, "default", "open", "owner", nil, true)
	fixtures.Volume(t, cluster, "default", "data", "member")
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues())

	// A Member writes the visibility of the applications it owns (KOSD-C-59).
	// The row judges the write, so the entry is offered to this role and the
	// rows it may not write stay read-only.
	member := userPage(t, shell, fixtures.Member)
	entry := itemLabel(t, member, "应用可见性").Entry
	if entry == nil || entry.Method != "PUT" {
		t.Fatalf("member entry for the visibility item = %+v, want the application write (KOSD-C-59)", entry)
	}
	if entry.Path != "/api/v1/resources/Application/{name}?namespace={namespace}" {
		t.Fatalf("member visibility entry = %q, want the application write", entry.Path)
	}
	rows := rowsOf(t, member, "应用可见性")
	if len(rows) != 2 {
		t.Fatalf("member visibility rows = %v, want the owned and the named application", rows)
	}
	if writable := rowMatching(t, rows, "name", "mine")["writable"]; writable != true {
		t.Fatalf("the member's own application row = %v, want it writable", rowMatching(t, rows, "name", "mine"))
	}
	if writable := rowMatching(t, rows, "name", "shared")["writable"]; writable != false {
		t.Fatalf("an application owned by another account = %v, want it read-only", rowMatching(t, rows, "name", "shared"))
	}
	// Ownership transfer stays the global Owner's (KOSD-C-60), and the account
	// list stays the Owner's write (KOSD-C-63): neither item carries an entry.
	if entry := itemLabel(t, member, "应用与卷所有者").Entry; entry != nil {
		t.Fatalf("member receives the transfer entry %+v, want none (KOSD-C-60)", entry)
	}
	if entry := itemLabel(t, member, "成员与角色").Entry; entry != nil {
		t.Fatalf("member receives the account entry %+v, want none (KOSD-C-63)", entry)
	}
	for _, action := range member.Actions {
		if action.ID != kossettings.ActionSetAppVisibility {
			t.Fatalf("member receives the action %q, want only the visibility write (KOSD-C-59)", action.ID)
		}
	}

	// The Owner writes every application row; a Guest reads the applications
	// open to guests and writes none of them (KOSD-C-59).
	owner := userPage(t, shell, fixtures.Owner)
	for _, row := range rowsOf(t, owner, "应用可见性") {
		if row["writable"] != true {
			t.Fatalf("owner row %v is not writable, want every row (KOSD-C-59)", row)
		}
	}
	guest := userPage(t, shell, fixtures.Guest)
	guestRows := rowsOf(t, guest, "应用可见性")
	if len(guestRows) != 1 || guestRows[0]["name"] != "open" {
		t.Fatalf("guest visibility rows = %v, want only the application open to guests", guestRows)
	}
	if guestRows[0]["writable"] != false {
		t.Fatalf("guest row %v is writable, want read-only (KOSD-C-59)", guestRows[0])
	}
}

func TestTC1InvisibleApplicationsStayOutOfTheProjection(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Application(t, cluster, "default", "private", "owner", nil, false)
	fixtures.Application(t, cluster, "default", "shared", "owner", []string{"member"}, false)
	fixtures.Application(t, cluster, "default", "open", "owner", nil, true)
	fixtures.Volume(t, cluster, "default", "data", "owner")
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues())

	// An application the session may not see is absent from both object-source
	// items, exactly as if the cluster did not hold it (KOSD-C-59, DASH-C-22).
	member := userPage(t, shell, fixtures.Member)
	for _, label := range []string{"应用可见性", "应用与卷所有者"} {
		for _, row := range rowsOf(t, member, label) {
			if row["name"] == "private" {
				t.Fatalf("member reads the application private in %q, want the rows it may not see hidden", label)
			}
		}
	}
	if names := namesOf(rowsOf(t, member, "应用可见性")); len(names) != 1 || names[0] != "shared" {
		t.Fatalf("member visibility rows = %v, want only the named application", names)
	}
	guest := userPage(t, shell, fixtures.Guest)
	if names := namesOf(rowsOf(t, guest, "应用可见性")); len(names) != 1 || names[0] != "open" {
		t.Fatalf("guest visibility rows = %v, want only the application open to guests", names)
	}
	if names := namesOf(rowsOf(t, guest, "应用与卷所有者")); len(names) != 2 {
		t.Fatalf("guest owner rows = %v, want the guest-visible application and the volume", names)
	}
	// Volumes have no visibility predicate: every signed-in role reads them
	// (KOSD-C-59).
	if kind := rowMatching(t, rowsOf(t, guest, "应用与卷所有者"), "name", "data")["kind"]; kind != "Volume" {
		t.Fatalf("guest volume row kind = %v, want Volume", kind)
	}
}

func TestTC1DatabaseOutageStatesTheReason(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Application(t, cluster, "default", "shop", "owner", nil, false)
	roster := fixtures.NewRoster(
		identity.Account{Username: "owner", Role: write.RoleOwner},
		identity.Account{Username: "member", Role: write.RoleMember},
	)
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues(), roster)

	roster.Down()

	// The page still reads while the database is down; the database item states
	// why it cannot list the accounts (SETTINGS-C-1, KOSD-C-61).
	page := userPage(t, shell, fixtures.Owner)
	value, ok := projectedValue(t, page, "成员与角色").(map[string]any)
	if !ok {
		t.Fatalf("member list value = %v, want the unavailable record", projectedValue(t, page, "成员与角色"))
	}
	if value["available"] != false {
		t.Fatalf("member list value = %v, want an unavailable item", value)
	}
	reason, _ := value["reason"].(string)
	if reason == "" || !strings.Contains(reason, "数据库") {
		t.Fatalf("member list reason = %q, want the database outage named", reason)
	}

	// The object sources are unaffected: the outage of the control-plane
	// database does not hide the cluster's objects (KOSD-C-61).
	if rows := rowsOf(t, page, "应用可见性"); len(rows) != 1 {
		t.Fatalf("visibility rows = %v, want the application the cluster holds", rows)
	}
	if rows := rowsOf(t, page, "应用与卷所有者"); len(rows) != 1 {
		t.Fatalf("owner rows = %v, want the application the cluster holds", rows)
	}

	// With the database back the same item lists the accounts again.
	roster.Up()
	page = userPage(t, shell, fixtures.Owner)
	if rows := rowsOf(t, page, "成员与角色"); len(rows) != 2 {
		t.Fatalf("member list after recovery = %v, want the two accounts", rows)
	}
}

func TestTC1ValuesFollowTheirOwnSource(t *testing.T) {
	cluster := fixtures.ObjectSet(t)
	fixtures.Application(t, cluster, "default", "shop", "owner", []string{"member"}, false)
	fixtures.Volume(t, cluster, "default", "data", "owner")
	roster := fixtures.NewRoster(
		identity.Account{Username: "owner", Role: write.RoleOwner},
		identity.Account{Username: "member", Role: write.RoleMember},
	)
	registry := fixtures.Install(t, fixtures.Product(t, "settings", time.Unix(1, 0)))
	shell := fixtures.NewShell(t, registry, cluster, fixtures.NewValues(), roster)

	before := userPage(t, shell, fixtures.Owner)
	ownersBefore := projectedValue(t, before, "应用与卷所有者")
	visibilityBefore := projectedValue(t, before, "应用可见性")

	// The object-source items follow the objects: a rewritten visibility and a
	// transferred owner show on the next read (SETTINGS-C-1, KOSD-C-59,
	// KOSD-C-60).
	fixtures.SetApplicationVisibility(t, cluster, "default", "shop", []string{"member", "guest"}, true)
	fixtures.SetObjectOwner(t, cluster, "Application", "default", "shop", "member")
	fixtures.Volume(t, cluster, "default", "backup", "member")

	afterObject := userPage(t, shell, fixtures.Owner)
	shop := rowMatching(t, rowsOf(t, afterObject, "应用可见性"), "name", "shop")
	if shop["allowGuests"] != true {
		t.Fatalf("visibility after the write = %v, want guests allowed", shop)
	}
	if listed, _ := shop["members"].([]any); len(listed) != 2 {
		t.Fatalf("visibility members after the write = %v, want both listed accounts", shop["members"])
	}
	owners := rowsOf(t, afterObject, "应用与卷所有者")
	if owner := rowMatching(t, owners, "name", "shop")["owner"]; owner != "member" {
		t.Fatalf("application owner after the transfer = %v, want the transferred account", owner)
	}
	if len(owners) != 3 {
		t.Fatalf("owner rows = %v, want the two volumes and the application", owners)
	}

	// The database item follows the database: a new account appears without
	// touching an object.
	roster.Add(identity.Account{Username: "auditor", Role: write.RoleGuest})
	afterDatabase := userPage(t, shell, fixtures.Owner)
	if rows := rowsOf(t, afterDatabase, "成员与角色"); len(rows) != 3 {
		t.Fatalf("member list after the account was added = %v, want three accounts", rows)
	}

	// A database change does not move the object-source items, and the object
	// changes above did not move the roster: each item judges one source only.
	if got := projectedValue(t, afterDatabase, "应用可见性"); !reflect.DeepEqual(got, projectedValue(t, afterObject, "应用可见性")) {
		t.Fatalf("visibility changed with the database: %v, want %v", got, projectedValue(t, afterObject, "应用可见性"))
	}
	if got := projectedValue(t, afterDatabase, "应用与卷所有者"); !reflect.DeepEqual(got, projectedValue(t, afterObject, "应用与卷所有者")) {
		t.Fatalf("ownership changed with the database: %v, want %v", got, projectedValue(t, afterObject, "应用与卷所有者"))
	}
	if reflect.DeepEqual(visibilityBefore, projectedValue(t, afterObject, "应用可见性")) {
		t.Fatal("the visibility item did not follow the application object")
	}
	if reflect.DeepEqual(ownersBefore, projectedValue(t, afterObject, "应用与卷所有者")) {
		t.Fatal("the ownership item did not follow the owned objects")
	}
}

func TestTC1UserPageDisappearsWithTheComponent(t *testing.T) {
	registry := fixtures.Install(t, fixtures.OtherProduct(t, time.Unix(2, 0)))
	for _, page := range registry.Pages() {
		if page.Title == userPageTitle {
			t.Fatalf("a product without kos-settings contributes the user page %+v (KOSD-C-28)", page)
		}
	}
}
