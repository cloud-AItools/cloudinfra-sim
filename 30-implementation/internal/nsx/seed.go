package nsx

import "fmt"

// seedPolicyLocked는 Phase 4b 보안·라우팅 시드다(WP 4.9). put은 Reset의 기록 함수다(메타데이터·경로 설정).
//
// 실제 NSX가 설치 시 만드는 객체 중 자체 시나리오에 필요한 것만 둔다. 이름·값은 NSX 문서의 기본 객체 이름을 따르되
// 구성 세부(서비스 항목, 기본 Rule 번호·순서)는 가정이다(BEHAVIOR_SPEC §11):
//   - Domain default(스펙 Policy 설명: "This object is created by default along with the Domain")
//   - 기본 Distributed Firewall 섹션 default-layer3-section과 기본 Rule default-layer3-rule(ALLOW, ANY)
//   - 기본 Service 일부(HTTP·HTTPS·SSH·DNS·DNS-UDP·ICMP-ALL): is_default=true(스펙: 바꾸거나 지울 수 없음)
//   - Edge Cluster(Fixture edge_clusters): Policy 실현 객체와 Manager EdgeCluster
//
// Fixture의 Tier-0·Group·Security Policy는 결정론적으로 만든다(FR-DM-0210).
func (e *Engine) seedPolicyLocked(put func(base, rel string, obj map[string]any)) {
	ns, seed := e.fix.Namespace, e.fix.Seed
	put(policyBase, "/infra/domains/default", map[string]any{"id": "default", "display_name": "default", "resource_type": "Domain"})

	for i, name := range e.fix.NSX.EdgeClusters {
		id := uuidFrom(ns, seed, "edge-cluster"+fmt.Sprint(i))
		put(managerBase, "/edge-clusters/"+id, map[string]any{
			"id": id, "display_name": name, "resource_type": "EdgeCluster", "member_node_type": "EDGE_NODE",
			"deployment_type": "VIRTUAL_MACHINE", "members": []any{},
		})
		put(policyBase, "/infra/sites/default/enforcement-points/default/edge-clusters/"+id, map[string]any{
			"id": id, "display_name": name, "resource_type": "PolicyEdgeCluster", "member_node_type": "EDGE_NODE", "nsx_id": id,
			"_system_owned": true,
		})
	}

	type svc struct {
		id    string
		entry map[string]any
	}
	l4 := func(proto, port string) map[string]any {
		return map[string]any{"resource_type": "L4PortSetServiceEntry", "l4_protocol": proto, "destination_ports": []any{port}, "source_ports": []any{}}
	}
	for _, s := range []svc{
		{"HTTP", l4("TCP", "80")}, {"HTTPS", l4("TCP", "443")}, {"SSH", l4("TCP", "22")},
		{"DNS", l4("TCP", "53")}, {"DNS-UDP", l4("UDP", "53")},
		{"ICMP-ALL", map[string]any{"resource_type": "ICMPTypeServiceEntry", "protocol": "ICMPv4"}},
	} {
		rel := "/infra/services/" + s.id
		put(policyBase, rel, map[string]any{"id": s.id, "display_name": s.id, "resource_type": "Service", "is_default": true, "_system_owned": true, "service_type": "NON_ETHER"})
		s.entry["id"], s.entry["display_name"] = s.id, s.id
		s.entry["_system_owned"] = true
		put(policyBase, rel+"/service-entries/"+s.id, s.entry)
	}

	const dl3 = "/infra/domains/default/security-policies/default-layer3-section"
	put(policyBase, dl3, map[string]any{
		"id": "default-layer3-section", "display_name": "Default Layer3 Section", "resource_type": "SecurityPolicy",
		"category": "Application", "is_default": true, "sequence_number": float64(2147483647), "stateful": true,
		"scope": []any{"ANY"},
	})
	put(policyBase, dl3+"/rules/default-layer3-rule", map[string]any{
		"id": "default-layer3-rule", "display_name": "Default Layer3 Rule", "resource_type": "Rule", "is_default": true,
		"rule_id": float64(2), "action": "ALLOW", "direction": "IN_OUT", "ip_protocol": "IPV4_IPV6", "logged": false, "disabled": false,
		"source_groups": []any{"ANY"}, "destination_groups": []any{"ANY"}, "services": []any{"ANY"}, "scope": []any{"ANY"},
		"sequence_number": float64(2147483647),
	})

	for _, t0 := range e.fix.NSX.Tier0s {
		put(policyBase, "/infra/tier-0s/"+t0, map[string]any{"id": t0, "display_name": t0, "resource_type": "Tier0", "ha_mode": "ACTIVE_STANDBY", "failover_mode": "NON_PREEMPTIVE"})
	}
	for i := 1; i <= e.fix.NSX.Groups; i++ {
		id := fmt.Sprintf("grp-%02d", i)
		put(policyBase, "/infra/domains/default/groups/"+id, map[string]any{
			"id": id, "display_name": id, "resource_type": "Group",
			"expression": []any{map[string]any{"resource_type": "Condition", "member_type": "VirtualMachine", "key": "Tag", "operator": "EQUALS", "value": "app|" + id}},
		})
	}
	for i := 1; i <= e.fix.NSX.SecurityPolicies; i++ {
		id := fmt.Sprintf("sp-%02d", i)
		rel := "/infra/domains/default/security-policies/" + id
		put(policyBase, rel, map[string]any{"id": id, "display_name": id, "resource_type": "SecurityPolicy", "category": "Application", "sequence_number": float64(i * 10), "stateful": true})
		src := []any{"ANY"}
		if i <= e.fix.NSX.Groups {
			src = []any{fmt.Sprintf("/infra/domains/default/groups/grp-%02d", i)}
		}
		e.ruleSeq++
		put(policyBase, rel+"/rules/rule-01", map[string]any{
			"id": "rule-01", "display_name": "rule-01", "resource_type": "Rule", "rule_id": float64(e.ruleSeq), "action": "ALLOW",
			"direction": "IN_OUT", "ip_protocol": "IPV4_IPV6", "logged": false, "disabled": false, "sequence_number": float64(10),
			"source_groups": src, "destination_groups": []any{"ANY"}, "services": []any{"/infra/services/HTTPS"}, "scope": []any{"ANY"},
		})
	}
}
