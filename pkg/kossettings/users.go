package kossettings

import (
	"context"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/tsic404/kosd/api/v1beta1"
	"github.com/tsic404/kosd/pkg/controlplane"
	"github.com/tsic404/kosd/pkg/identity"
	"github.com/tsic404/kosd/pkg/write"
)

// DatabaseUnavailableReason is what the member list states while the
// control-plane database does not answer (C-1, KOSD-C-61).
const DatabaseUnavailableReason = "控制面数据库不可用，成员与角色暂不可读"

// MemberSource reads the accounts the control-plane database holds (C-1,
// KOSD-C-53). The read is not role-gated: every signed-in account reads the
// user page, and only the writes converge by role (KOSD-C-59).
type MemberSource interface {
	// ListAccounts returns every account the database holds.
	ListAccounts(ctx context.Context) ([]identity.Account, error)
}

// memberRole is one account row of the member list: the account, the role it
// currently holds and whether it is disabled (C-1, KOSD-C-63).
type memberRole struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled,omitempty"`
}

// appVisibility is one application row of the visibility item: the application,
// the visibility its spec declares, and whether this session may write it
// (C-1, KOSD-C-59).
type appVisibility struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Members     []string `json:"members"`
	AllowGuests bool     `json:"allowGuests"`
	IsWritable  bool     `json:"writable"`
}

// ownedObject is one application or volume row of the ownership item: the
// object and the account its spec records as owner (C-1, KOSD-C-60).
type ownedObject struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
}

// memberRoles reads the member list from the control-plane database. A database
// that does not answer leaves the item stating the reason instead of failing
// the page: the other two items read cluster objects and stay readable
// (C-1, KOSD-C-61).
func (r *Readings) memberRoles(ctx context.Context) (any, *controlplane.Error) {
	if r.Members == nil {
		return unavailableValue(DatabaseUnavailableReason), nil
	}
	accounts, err := r.Members.ListAccounts(ctx)
	if err != nil {
		return unavailableValue(DatabaseUnavailableReason), nil
	}
	rows := make([]memberRole, 0, len(accounts))
	for _, account := range accounts {
		rows = append(rows, memberRole{
			Username: account.Username,
			Role:     string(account.Role),
			Disabled: account.Disabled,
		})
	}
	sort.Slice(rows, func(left, right int) bool { return rows[left].Username < rows[right].Username })
	return rows, nil
}

// appVisibility reads every application the session may see, with the
// visibility its spec declares and whether this session writes it (C-1,
// KOSD-C-59).
func (r *Readings) appVisibility(ctx context.Context, session identity.Session) (any, *controlplane.Error) {
	objects, failure := r.kernel(ctx, "Application")
	if failure != nil {
		return nil, failure
	}
	reader := r.reader()
	rows := make([]appVisibility, 0, len(objects))
	for _, object := range objects {
		visible, failure := reader.Visible(ctx, session, "Application", object.Object)
		if failure != nil {
			return nil, failure
		}
		if !visible {
			continue
		}
		var application v1beta1.Application
		if failure := decode(object, &application); failure != nil {
			return nil, failure
		}
		members := make([]string, 0, len(application.Spec.Visibility.Members))
		members = append(members, application.Spec.Visibility.Members...)
		sort.Strings(members)
		rows = append(rows, appVisibility{
			Namespace:   application.Namespace,
			Name:        application.Name,
			Members:     members,
			AllowGuests: application.Spec.Visibility.AllowGuests,
			IsWritable:  mayWriteApplication(session, application),
		})
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Namespace != rows[right].Namespace {
			return rows[left].Namespace < rows[right].Namespace
		}
		return rows[left].Name < rows[right].Name
	})
	return rows, nil
}

// mayWriteApplication applies KOSD-C-59 to one visibility row: the global Owner
// writes every application, a Member only the applications it owns, a Guest
// none.
func mayWriteApplication(session identity.Session, application v1beta1.Application) bool {
	switch session.Role {
	case write.RoleOwner:
		return true
	case write.RoleMember:
		return application.Spec.Owner != "" && application.Spec.Owner == session.Username
	default:
		return false
	}
}

// appAndVolumeOwners reads the owner each application and volume the session
// may see records in its spec (C-1, KOSD-C-59, KOSD-C-60).
func (r *Readings) appAndVolumeOwners(ctx context.Context, session identity.Session) (any, *controlplane.Error) {
	reader := r.reader()
	rows := make([]ownedObject, 0, 8)
	for _, kind := range []string{"Application", "Volume"} {
		objects, failure := r.kernel(ctx, kind)
		if failure != nil {
			return nil, failure
		}
		for _, object := range objects {
			visible, failure := reader.Visible(ctx, session, kind, object.Object)
			if failure != nil {
				return nil, failure
			}
			if !visible {
				continue
			}
			owner, failure := objectOwner(object, kind)
			if failure != nil {
				return nil, failure
			}
			rows = append(rows, ownedObject{
				Kind: kind, Namespace: object.GetNamespace(), Name: object.GetName(), Owner: owner,
			})
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Kind != rows[right].Kind {
			return rows[left].Kind < rows[right].Kind
		}
		if rows[left].Namespace != rows[right].Namespace {
			return rows[left].Namespace < rows[right].Namespace
		}
		return rows[left].Name < rows[right].Name
	})
	return rows, nil
}

// objectOwner reads the spec.owner of one owned kind (C-60).
func objectOwner(object unstructured.Unstructured, kind string) (string, *controlplane.Error) {
	switch kind {
	case "Application":
		var application v1beta1.Application
		if failure := decode(object, &application); failure != nil {
			return "", failure
		}
		return application.Spec.Owner, nil
	case "Volume":
		var volume v1beta1.Volume
		if failure := decode(object, &volume); failure != nil {
			return "", failure
		}
		return volume.Spec.Owner, nil
	default:
		return "", unreadable(kind + " 不是带所有者的对象类型")
	}
}

// unavailableValue is the shape an item uses to state why it shows no value;
// the page renders the reason instead of inventing one (C-1, C-71).
func unavailableValue(reason string) map[string]any {
	return map[string]any{"available": false, "reason": reason}
}
