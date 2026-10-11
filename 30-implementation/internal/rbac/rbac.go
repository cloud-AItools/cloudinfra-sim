// Package rbac는 자체 권한 시나리오용 역할 모델이다(PRD-1 FR-API-0211 ~ 0213, WP 4.2).
//
// 실제 VMware 권한 모델 전체가 아니라 테스트 클라이언트가 검증하는 시나리오를 지원한다(FR-API-0213):
//   - vCenter: 메서드별 권한 ID는 공식 타입 정보(SRC-VIM-TYPEINFO, pyVmomi 메서드 privilege)에서 가져오고,
//     역할은 vSphere 기본 역할 이름(Admin, ReadOnly)과 시뮬레이터 시험용 VirtualMachineOperator(가정)를 둔다.
//     범위(scope): 사용자에게 엔터티 MoRef를 주면 그 엔터티와 하위에서만 권한이 있다(System.* 권한 제외).
//   - NSX: 기본 역할 이름 enterprise_admin(전체)·auditor(조회만).
package rbac

import "strings"

const (
	Admin              = "Admin"
	ReadOnly           = "ReadOnly"
	VMOperator         = "VirtualMachineOperator"
	NSXEnterpriseAdmin = "enterprise_admin"
	NSXAuditor         = "auditor"
)

// Valid는 제품에 맞는 역할 이름인가. 빈 역할은 기본(전체 권한)이다.
func Valid(product, role string) bool {
	switch product {
	case "vcenter":
		return role == "" || role == Admin || role == ReadOnly || role == VMOperator
	case "nsx":
		return role == "" || role == NSXEnterpriseAdmin || role == NSXAuditor
	}
	return false
}

// IsSystem은 범위와 무관한 시스템 권한인가.
func IsSystem(priv string) bool { return strings.HasPrefix(priv, "System.") }

// VCenterAllows는 vCenter 역할이 권한 ID를 가지는가. 권한을 모르는 메서드는 Admin만 허용한다.
func VCenterAllows(role, priv string) bool {
	switch role {
	case "", Admin:
		return true
	case ReadOnly:
		return priv == "System.Anonymous" || priv == "System.View" || priv == "System.Read"
	case VMOperator:
		return IsSystem(priv) || strings.HasPrefix(priv, "VirtualMachine.Interact.") || strings.HasPrefix(priv, "VirtualMachine.Config.")
	}
	return false
}

// NSXAllows는 NSX 역할이 HTTP 메서드를 쓸 수 있는가.
func NSXAllows(role, method string) bool {
	if role == NSXAuditor {
		return method == "GET" || method == "HEAD"
	}
	return true
}
