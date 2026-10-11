package vcenter

import (
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/rbac"
)

// checkPermission은 사용자 역할·범위로 메서드 권한을 판정한다(FR-API-0211·0212). 권한 부족은 NoPermission
// (privilegeId 포함, vSphere 계약). 권한 ID는 공식 타입 정보(profile.VersionMeta.Privilege)에서 찾는다.
func (v *VCenter) checkPermission(ctx *simulator.Context, method *simulator.Method) types.BaseMethodFault {
	if v.Role == nil || ctx.Session == nil || ctx.Session.UserName == "" {
		return nil
	}
	role, scope := v.Role("vcenter", ctx.Session.UserName)
	if role == "" && scope == "" {
		return nil
	}
	priv := v.prof.Meta.Privilege(method.This.Type, method.Name)
	if priv == "" {
		// 권한 정보가 없는 메서드: 인벤토리 엔터티가 아닌 세션 객체(PropertyFilter, View 등)는 허용(가정), 엔터티는 Admin만
		if _, entity := ctx.Map.Get(method.This).(mo.Entity); !entity {
			return nil
		}
	}
	ok := rbac.VCenterAllows(role, priv)
	if ok && scope != "" && !rbac.IsSystem(priv) && priv != "" {
		ok = v.under(ctx, method.This, scope)
	}
	if ok {
		return nil
	}
	this := method.This
	v.log.Info("permission denied", "user", ctx.Session.UserName, "role", role, "scope", scope, "method", method.Name, "object", this.Value, "privilege", priv)
	return &types.NoPermission{Object: &this, PrivilegeId: priv}
}

// under는 엔터티 ref가 scope 엔터티이거나 그 하위인가(인벤토리 부모를 따라 올라간다).
func (v *VCenter) under(ctx *simulator.Context, ref types.ManagedObjectReference, scope string) bool {
	for i := 0; i < 64; i++ {
		if ref.Value == scope {
			return true
		}
		obj := ctx.Map.Get(ref)
		e, ok := obj.(mo.Entity)
		if !ok {
			return false
		}
		var parent *types.ManagedObjectReference
		ctx.WithLock(obj, func() { parent = e.Entity().Parent })
		if parent == nil {
			return false
		}
		ref = *parent
	}
	return false
}
