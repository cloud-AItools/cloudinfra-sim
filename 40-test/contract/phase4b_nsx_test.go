// Phase 4b NSX 기능 확장 계약(WP 4.9). 근거: SRC-NSX-321 오퍼레이션·필드 설명. 가정 항목은 BEHAVIOR_SPEC §11.
package contract

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/find"

	"cloudinfra-sim/test/internal/nsxcheck"
	"cloudinfra-sim/test/internal/testenv"
)

const p4 = "/policy/api/v1"

func mustNSX(t *testing.T, e *testenv.Env, method, path, body string, want int) map[string]any {
	t.Helper()
	code, out := nsxDo(t, e, method, path, body)
	if code != want {
		t.Fatalf("%s %s: %d, want %d: %v", method, path, code, want, out)
	}
	return out
}

func results(o map[string]any) []map[string]any {
	var out []map[string]any
	rs, _ := o["results"].([]any)
	for _, r := range rs {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func ids(items []map[string]any, field string) []string {
	var out []string
	for _, it := range items {
		out = append(out, fmt.Sprint(it[field]))
	}
	return out
}

// T-FR-NSX-0215 T-FR-NSX-0216 T-FR-NSX-0217 T-FR-NSX-0218 T-FR-NSX-0410 T-FR-NSX-0415
func TestNSXSecurityResources(t *testing.T) {
	e := testenv.Start(t, nil)
	// 시드: 기본 Domain·기본 섹션·기본 Service·Fixture Group/Policy
	mustNSX(t, e, "GET", p4+"/infra/domains/default", "", 200)
	dl3 := mustNSX(t, e, "GET", p4+"/infra/domains/default/security-policies/default-layer3-section", "", 200)
	if dl3["is_default"] != true || dl3["rule_count"] != float64(1) {
		t.Fatalf("default section: %v", dl3)
	}
	if n := len(results(mustNSX(t, e, "GET", p4+"/infra/domains/default/groups", "", 200))); n != 5 {
		t.Fatalf("fixture groups: %d", n)
	}
	https := mustNSX(t, e, "GET", p4+"/infra/services/HTTPS", "", 200)
	if https["is_default"] != true || len(https["service_entries"].([]any)) != 1 {
		t.Fatalf("default service HTTPS: %v", https)
	}
	// 기본 Service는 바꾸거나 지울 수 없다(스펙 is_default 설명)
	mustNSX(t, e, "PATCH", p4+"/infra/services/HTTPS", `{"display_name":"x"}`, 403)
	mustNSX(t, e, "DELETE", p4+"/infra/services/HTTPS", "", 403)

	// Service + 내장 service_entries. 다형 하위 타입(L4PortSetServiceEntry)의 필수 l4_protocol 검사
	mustNSX(t, e, "PATCH", p4+"/infra/services/svc-app", `{"service_entries":[{"resource_type":"L4PortSetServiceEntry","id":"tcp-8443","destination_ports":["8443"]}]}`, 400)
	mustNSX(t, e, "PATCH", p4+"/infra/services/svc-app", `{"display_name":"app","service_entries":[{"resource_type":"L4PortSetServiceEntry","id":"tcp-8443","l4_protocol":"TCP","destination_ports":["8443","9000-9010"]}]}`, 200)
	svc := mustNSX(t, e, "GET", p4+"/infra/services/svc-app", "", 200)
	if es := svc["service_entries"].([]any); len(es) != 1 || es[0].(map[string]any)["path"] != "/infra/services/svc-app/service-entries/tcp-8443" {
		t.Fatalf("service entries: %v", svc)
	}

	// Group: expression 규칙(스펙) 위반은 400
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/web", `{"expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Tag","operator":"EQUALS","value":"tier|web"},{"resource_type":"Condition","member_type":"VirtualMachine","key":"Name","operator":"EQUALS","value":"x"}]}`, 400)
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/web", `{"expression":[{"resource_type":"Condition","member_type":"VirtualMachine","key":"Tag","operator":"EQUALS","value":"tier|web"}]}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/db", `{"expression":[{"resource_type":"IPAddressExpression","ip_addresses":["10.0.0.10","10.0.1.0/24"]}]}`, 200)

	// Security Policy + 내장 rules
	const sp = p4 + "/infra/domains/default/security-policies/app"
	rule := func(id string, seq int, src, svc string) string {
		return fmt.Sprintf(`{"id":%q,"action":"ALLOW","sequence_number":%d,"source_groups":[%q],"destination_groups":["/infra/domains/default/groups/db"],"services":[%q],"scope":["ANY"]}`, id, seq, src, svc)
	}
	// 참조 대상이 없으면 400, "ANY"는 단독이어야 한다
	mustNSX(t, e, "PATCH", sp, `{"category":"Application","rules":[`+rule("r1", 10, "/infra/domains/default/groups/none", "/infra/services/svc-app")+`]}`, 400)
	mustNSX(t, e, "PATCH", sp, `{"category":"Application","rules":[{"id":"r1","action":"ALLOW","services":["ANY","/infra/services/HTTP"]}]}`, 400)
	mustNSX(t, e, "GET", sp, "", 404) // 실패한 요청은 아무것도 남기지 않는다
	mustNSX(t, e, "PATCH", sp, `{"category":"Application","rules":[`+rule("r2", 20, "/infra/domains/default/groups/web", "/infra/services/HTTPS")+`,`+rule("r1", 10, "/infra/domains/default/groups/web", "/infra/services/svc-app")+`]}`, 200)
	pol := mustNSX(t, e, "GET", sp, "", 200)
	rs := pol["rules"].([]any)
	if len(rs) != 2 || rs[0].(map[string]any)["id"] != "r1" || pol["rule_count"] != float64(2) {
		t.Fatalf("embedded rules (sequence order): %v", pol)
	}
	r1 := rs[0].(map[string]any)
	if id, _ := r1["rule_id"].(float64); id <= 1000 || r1["direction"] != "IN_OUT" || r1["path"] != "/infra/domains/default/security-policies/app/rules/r1" {
		t.Fatalf("rule system fields/defaults: %v", r1)
	}
	// Rule 단독 수정: Revision 증가, rule_id 유지, 오래된 Revision은 412
	mustNSX(t, e, "PATCH", sp+"/rules/r1", `{"action":"DROP","sequence_number":10,"source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"],"_revision":0}`, 200)
	got := mustNSX(t, e, "GET", sp+"/rules/r1", "", 200)
	if got["_revision"] != float64(1) || got["action"] != "DROP" || got["rule_id"] != r1["rule_id"] {
		t.Fatalf("rule update: %v", got)
	}
	mustNSX(t, e, "PATCH", sp+"/rules/r1", `{"action":"ALLOW","_revision":0}`, 412)
	// 참조 중인 Group·Service 삭제 거부
	mustNSX(t, e, "DELETE", p4+"/infra/domains/default/groups/web", "", 400)
	// revise: r1을 맨 아래로
	mustNSX(t, e, "POST", sp+"/rules/r1?action=revise&operation=insert_bottom", `{"action":"DROP","source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"]}`, 200)
	list := results(mustNSX(t, e, "GET", sp+"/rules", "", 200))
	if strings.Join(ids(list, "id"), ",") != "r2,r1" {
		t.Fatalf("revise insert_bottom: %v", ids(list, "id"))
	}
	mustNSX(t, e, "POST", sp+"/rules/r1?action=revise&operation=insert_before&anchor_path=/infra/domains/default/security-policies/app/rules/r2", `{"action":"DROP","source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"]}`, 200)
	if list := results(mustNSX(t, e, "GET", sp+"/rules", "", 200)); strings.Join(ids(list, "id"), ",") != "r1,r2" {
		t.Fatalf("revise insert_before: %v", ids(list, "id"))
	}
	// PUT: 전체 교체(나열하지 않은 Rule 삭제), 200 + 객체
	cur := mustNSX(t, e, "GET", sp, "", 200)
	put := mustNSX(t, e, "PUT", sp, fmt.Sprintf(`{"category":"Application","_revision":%v,"rules":[%s]}`, cur["_revision"], rule("r3", 5, "ANY", "ANY")), 200)
	if ids := ids(toMaps(put["rules"]), "id"); strings.Join(ids, ",") != "r3" {
		t.Fatalf("PUT replace rules: %v", put)
	}
	mustNSX(t, e, "GET", sp+"/rules/r1", "", 404)
	// 정책 삭제는 Rule도 지운다. 그 뒤 Group은 지울 수 있다
	mustNSX(t, e, "DELETE", sp, "", 200)
	mustNSX(t, e, "GET", sp+"/rules/r3", "", 404)
	mustNSX(t, e, "DELETE", p4+"/infra/domains/default/groups/web", "", 200)
	// 시뮬레이터가 구현하지 않은 action은 저장하지 않고 501
	mustNSX(t, e, "POST", p4+"/infra/tier-1s/t1-a?action=reprocess", ``, 501)
}

// disconnectHost는 이름이 name인 호스트의 연결을 끊는다(HostSystem.DisconnectHost_Task).
func disconnectHost(t *testing.T, e *testenv.Env, name string) {
	t.Helper()
	ctx := context.Background()
	c := e.Login("")
	f := find.NewFinder(c.Client, true)
	dc, err := f.DefaultDatacenter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.SetDatacenter(dc)
	hosts, err := f.HostSystemList(ctx, "*")
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hosts {
		if h.Name() == name {
			tk, err := h.Disconnect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := tk.Wait(ctx); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("host %s not found", name)
}

func toMaps(v any) []map[string]any {
	var out []map[string]any
	a, _ := v.([]any)
	for _, x := range a {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// T-FR-NSX-0212 T-FR-NSX-0213 T-FR-NSX-0214 T-FR-NSX-0507
func TestNSXRoutingResources(t *testing.T) {
	e := testenv.Start(t, nil)
	mustNSX(t, e, "GET", p4+"/infra/tier-0s/t0-a", "", 200)
	ecs := results(mustNSX(t, e, "GET", p4+"/infra/sites/default/enforcement-points/default/edge-clusters", "", 200))
	if len(ecs) != 1 {
		t.Fatalf("edge clusters: %v", ecs)
	}
	ec := ecs[0]["path"].(string)
	// Tier-0 locale services(edge cluster 참조), Tier-1(Tier-0 참조), Segment(Tier-1 연결), Tier-1 Interface 없이 Service Interface
	mustNSX(t, e, "PATCH", p4+"/infra/tier-0s/t0-a/locale-services/default", `{"edge_cluster_path":"`+ec+`"}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/tier-0s/t0-a/locale-services/bad", `{"edge_cluster_path":"/infra/sites/default/enforcement-points/default/edge-clusters/none"}`, 400)
	mustNSX(t, e, "PATCH", p4+"/infra/tier-1s/t1-app", `{"tier0_path":"/infra/tier-0s/t0-a","route_advertisement_types":["TIER1_CONNECTED"]}`, 200)
	t1 := mustNSX(t, e, "GET", p4+"/infra/tier-1s/t1-app", "", 200)
	if t1["tier0_path"] != "/infra/tier-0s/t0-a" {
		t.Fatalf("tier1: %v", t1)
	}
	mustNSX(t, e, "PATCH", p4+"/infra/tier-1s/t1-app/locale-services/default", `{"edge_cluster_path":"`+ec+`"}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/uplink", `{"transport_zone_path":"/infra/sites/default/enforcement-points/default/transport-zones/tz-vlan","vlan_ids":["100"]}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/tier-0s/t0-a/locale-services/default/interfaces/uplink-1", `{"segment_path":"/infra/segments/uplink","type":"EXTERNAL","subnets":[{"ip_addresses":["192.168.100.2"],"prefix_len":24}]}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/tier-0s/t0-a/locale-services/default/interfaces/bad", `{"segment_path":"/infra/segments/none","subnets":[{"ip_addresses":["192.168.100.3"],"prefix_len":24}]}`, 400)
	ifs := results(mustNSX(t, e, "GET", p4+"/infra/tier-0s/t0-a/locale-services/default/interfaces", "", 200))
	if len(ifs) != 1 || ifs[0]["segment_path"] != "/infra/segments/uplink" {
		t.Fatalf("tier0 interfaces: %v", ifs)
	}
	// 참조 중인 Tier-0·Segment는 지울 수 없다. 하위(locale-services)가 있는 Tier-1도 거부
	mustNSX(t, e, "DELETE", p4+"/infra/tier-0s/t0-a", "", 400)
	mustNSX(t, e, "DELETE", p4+"/infra/segments/uplink", "", 400)
	mustNSX(t, e, "DELETE", p4+"/infra/tier-1s/t1-app", "", 400)
	// Realization: Interface는 Segment가 실현된 뒤 SUCCESS(의존, 가정)
	waitRealized(t, e, "/infra/tier-0s/t0-a/locale-services/default/interfaces/uplink-1")
	// 정리 순서대로 지운다
	for _, p := range []string{"/infra/tier-0s/t0-a/locale-services/default/interfaces/uplink-1", "/infra/segments/uplink", "/infra/tier-1s/t1-app/locale-services/default", "/infra/tier-1s/t1-app"} {
		mustNSX(t, e, "DELETE", p4+p, "", 200)
	}
}

func waitRealized(t *testing.T, e *testenv.Env, intent string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, st := nsxDo(t, e, "GET", p4+"/infra/realized-state/status?intent_path="+intent, "")
		cs, _ := st["consolidated_status"].(map[string]any)
		if cs["consolidated_status"] == "SUCCESS" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s not realized: %v", intent, st)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// T-FR-NSX-0223 T-FR-NSX-0224 T-FR-NSX-0225
func TestNSXHierarchicalAPI(t *testing.T) {
	e := testenv.Start(t, nil)
	// 한 요청: Domain 참조 아래 Group·Policy·Rule, 최상위 Service. Rule이 같은 요청의 Service·Group을 참조하며
	// 참조 대상이 뒤에 나와도 처리된다(FR-NSX-0224).
	body := `{"resource_type":"Infra","children":[
	 {"resource_type":"ChildResourceReference","id":"default","target_type":"Domain","children":[
	   {"resource_type":"ChildSecurityPolicy","SecurityPolicy":{"resource_type":"SecurityPolicy","id":"h-pol","category":"Application","children":[
	     {"resource_type":"ChildRule","Rule":{"resource_type":"Rule","id":"h-rule","action":"ALLOW","source_groups":["/infra/domains/default/groups/h-grp"],"destination_groups":["ANY"],"services":["/infra/services/h-svc"]}}]}},
	   {"resource_type":"ChildGroup","Group":{"resource_type":"Group","id":"h-grp","expression":[{"resource_type":"IPAddressExpression","ip_addresses":["10.9.0.0/16"]}]}}]},
	 {"resource_type":"ChildService","Service":{"resource_type":"Service","id":"h-svc","service_entries":[{"resource_type":"L4PortSetServiceEntry","id":"e1","l4_protocol":"UDP","destination_ports":["5000"]}]}},
	 {"resource_type":"ChildTier1","Tier1":{"resource_type":"Tier1","id":"h-t1","tier0_path":"/infra/tier-0s/t0-a"}},
	 {"resource_type":"ChildSegment","Segment":{"resource_type":"Segment","id":"h-seg","connectivity_path":"/infra/tier-1s/h-t1"}}]}`
	mustNSX(t, e, "PATCH", p4+"/infra", body, 200)
	for _, p := range []string{"/infra/domains/default/groups/h-grp", "/infra/domains/default/security-policies/h-pol/rules/h-rule", "/infra/services/h-svc/service-entries/e1", "/infra/tier-1s/h-t1", "/infra/segments/h-seg"} {
		mustNSX(t, e, "GET", p4+p, "", 200)
	}
	// 원자성(가정, FR-NSX-0225): 자식 하나가 잘못되면(없는 참조) 아무것도 반영하지 않는다
	bad := `{"resource_type":"Infra","children":[
	 {"resource_type":"ChildSegment","Segment":{"resource_type":"Segment","id":"h-ok"}},
	 {"resource_type":"ChildSegment","Segment":{"resource_type":"Segment","id":"h-bad","connectivity_path":"/infra/tier-1s/none"}}]}`
	mustNSX(t, e, "PATCH", p4+"/infra", bad, 400)
	mustNSX(t, e, "GET", p4+"/infra/segments/h-ok", "", 404)
	// 스키마 위반(다형 Child 래퍼 안의 enum)도 400
	mustNSX(t, e, "PATCH", p4+"/infra", `{"resource_type":"Infra","children":[{"resource_type":"ChildSegment","Segment":{"resource_type":"Segment","id":"h-x","admin_state":"SIDEWAYS"}}]}`, 400)
	// enforce_revision_check=true면 _revision을 검사한다(기본은 검사 안 함)
	seg := mustNSX(t, e, "GET", p4+"/infra/segments/h-seg", "", 200)
	stale := `{"resource_type":"Infra","children":[{"resource_type":"ChildSegment","Segment":{"resource_type":"Segment","id":"h-seg","connectivity_path":"/infra/tier-1s/h-t1","_revision":99}}]}`
	mustNSX(t, e, "PATCH", p4+"/infra?enforce_revision_check=true", stale, 412)
	mustNSX(t, e, "PATCH", p4+"/infra", stale, 200)
	if after := mustNSX(t, e, "GET", p4+"/infra/segments/h-seg", "", 200); after["_revision"] != seg["_revision"].(float64)+1 {
		t.Fatalf("revision after H-API update: %v", after["_revision"])
	}
	// marked_for_delete: 하위 트리와 함께 삭제(Policy → Rule). 다른 객체가 참조 중인 대상은 같은 요청에서 참조도 지우면 성공
	del := `{"resource_type":"Infra","children":[
	 {"resource_type":"ChildService","marked_for_delete":true,"Service":{"resource_type":"Service","id":"h-svc"}},
	 {"resource_type":"ChildResourceReference","id":"default","target_type":"Domain","children":[
	   {"resource_type":"ChildSecurityPolicy","marked_for_delete":true,"SecurityPolicy":{"resource_type":"SecurityPolicy","id":"h-pol"}}]}]}`
	mustNSX(t, e, "PATCH", p4+"/infra", del, 200)
	mustNSX(t, e, "GET", p4+"/infra/domains/default/security-policies/h-pol/rules/h-rule", "", 404)
	mustNSX(t, e, "GET", p4+"/infra/services/h-svc", "", 404)
	// 참조가 남아 있으면 삭제 거부(Tier-1은 Segment가 참조)
	mustNSX(t, e, "PATCH", p4+"/infra", `{"resource_type":"Infra","children":[{"resource_type":"ChildTier1","marked_for_delete":true,"Tier1":{"resource_type":"Tier1","id":"h-t1"}}]}`, 400)
	// 없는 대상에 대한 ChildResourceReference는 400
	mustNSX(t, e, "PATCH", p4+"/infra", `{"resource_type":"Infra","children":[{"resource_type":"ChildResourceReference","id":"nope","target_type":"Domain","children":[]}]}`, 400)
}

// T-FR-NSX-0221 T-FR-NSX-0232 T-FR-NSX-0413
func TestNSXSearchAndFilters(t *testing.T) {
	e := testenv.Start(t, nil)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/web-01", `{"tags":[{"scope":"app","tag":"web"}]}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/segments/web-02", `{"tags":[{"scope":"app","tag":"web"}],"admin_state":"DOWN"}`, 200)
	q := func(query string, extra string) []map[string]any {
		return results(mustNSX(t, e, "GET", p4+"/search/query?query="+strings.ReplaceAll(query, " ", "%20")+extra, "", 200))
	}
	if got := ids(q("resource_type:Segment AND tags.tag:web", ""), "id"); strings.Join(got, ",") != "web-01,web-02" {
		t.Fatalf("tag search: %v", got)
	}
	if got := ids(q("resource_type:Segment AND display_name:web* AND NOT admin_state:DOWN", ""), "id"); strings.Join(got, ",") != "web-01" {
		t.Fatalf("boolean search: %v", got)
	}
	if got := q("resource_type:Segment", "&page_size=4"); len(got) != 4 {
		t.Fatalf("paging: %d", len(got))
	}
	page := mustNSX(t, e, "GET", p4+"/search/query?query=resource_type:Segment&page_size=4", "", 200)
	if page["result_count"] != float64(12) || page["cursor"] != "4" {
		t.Fatalf("result_count/cursor: %v %v", page["result_count"], page["cursor"])
	}
	if got := ids(q("resource_type:Segment", "&sort_by=display_name&sort_ascending=false&page_size=1"), "id"); got[0] != "web-02" {
		t.Fatalf("sort: %v", got)
	}
	if got := q("resource_type:Service AND is_default:true", ""); len(got) != 6 {
		t.Fatalf("default services: %d", len(got))
	}
	mustNSX(t, e, "GET", p4+"/search/query?query=display_name:[a%20TO%20b]", "", 400)
	mustNSX(t, e, "GET", p4+"/search/query?query=", "", 400)
	// Manager 검색
	if got := results(mustNSX(t, e, "GET", "/api/v1/search/query?query=resource_type:TransportZone", "", 200)); len(got) != 2 {
		t.Fatalf("manager search: %d", len(got))
	}
	// 목록 필터(같음 조건)
	if got := results(mustNSX(t, e, "GET", p4+"/infra/segments?admin_state=DOWN", "", 200)); len(got) != 1 || got[0]["id"] != "web-02" {
		t.Fatalf("list filter: %v", got)
	}
}

// T-FR-NSX-0229 T-FR-NSX-0605
func TestNSXGroupMembership(t *testing.T) {
	e := testenv.Start(t, nil)
	attachVMToSegment(t, e, "/infra/segments/seg-01")
	vms := results(mustNSX(t, e, "GET", "/api/v1/fabric/virtual-machines", "", 200))
	if len(vms) < 2 {
		t.Fatalf("fabric vms: %d", len(vms))
	}
	ext := vms[0]["external_id"].(string)
	// Fixture Group grp-01은 VM Tag app|grp-01 조건. 태그를 붙이면 멤버가 된다(대소문자 무관 평가)
	mustNSX(t, e, "POST", "/api/v1/fabric/virtual-machines?action=add_tags", `{"external_id":"`+ext+`","tags":[{"scope":"APP","tag":"GRP-01"}]}`, 204)
	m := results(mustNSX(t, e, "GET", p4+"/infra/domains/default/groups/grp-01/members/virtual-machines", "", 200))
	if len(m) != 1 || m[0]["id"] != ext {
		t.Fatalf("vm members: %v", m)
	}
	// Segment·SegmentPort 태그 조건(operator 생략 가능), 중첩 Group(PathExpression), IP 멤버
	mustNSX(t, e, "PATCH", p4+"/infra/segments/seg-02", `{"tags":[{"scope":"zone","tag":"dmz"}]}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/dmz-seg", `{"expression":[{"resource_type":"Condition","member_type":"Segment","key":"Tag","value":"zone|dmz"}]}`, 200)
	segs := results(mustNSX(t, e, "GET", p4+"/infra/domains/default/groups/dmz-seg/members/segments", "", 200))
	if len(segs) != 1 || segs[0]["path"] != "/infra/segments/seg-02" {
		t.Fatalf("segment members: %v", segs)
	}
	ports := results(mustNSX(t, e, "GET", p4+"/infra/segments/seg-01/ports", "", 200))
	if len(ports) != 1 {
		t.Fatalf("ports: %v", ports)
	}
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/groups/nested", `{"expression":[{"resource_type":"PathExpression","paths":["/infra/domains/default/groups/dmz-seg","`+ports[0]["path"].(string)+`"]},{"resource_type":"ConjunctionOperator","conjunction_operator":"OR"},{"resource_type":"IPAddressExpression","ip_addresses":["10.0.0.5"]}]}`, 200)
	if got := results(mustNSX(t, e, "GET", p4+"/infra/domains/default/groups/nested/members/segments", "", 200)); len(got) != 1 {
		t.Fatalf("nested group segments: %v", got)
	}
	if got := results(mustNSX(t, e, "GET", p4+"/infra/domains/default/groups/nested/members/segment-ports", "", 200)); len(got) != 1 {
		t.Fatalf("path segment-ports: %v", got)
	}
	ips := mustNSX(t, e, "GET", p4+"/infra/domains/default/groups/nested/members/ip-addresses", "", 200)
	if fmt.Sprint(ips["results"]) != "[10.0.0.5]" {
		t.Fatalf("ip members: %v", ips)
	}
	// 참조 중인 Port 경로가 있어도 시스템 소유 Port는 사용자가 지울 수 없다(403)
	mustNSX(t, e, "DELETE", p4+ports[0]["path"].(string), "", 403)
}

// T-FR-NSX-0206 T-FR-NSX-0207 T-FR-NSX-0208
func TestNSXManagerInventory(t *testing.T) {
	e := testenv.Start(t, nil)
	tns := results(mustNSX(t, e, "GET", "/api/v1/transport-nodes", "", 200))
	if len(tns) == 0 {
		t.Fatal("no transport nodes")
	}
	tn := tns[0]
	sw := tn["host_switch_spec"].(map[string]any)["host_switches"].([]any)[0].(map[string]any)
	if sw["host_switch_type"] != "VDS" || len(sw["transport_zone_endpoints"].([]any)) != 2 {
		t.Fatalf("transport node host switch: %v", sw)
	}
	st := mustNSX(t, e, "GET", "/api/v1/transport-nodes/"+tn["id"].(string)+"/state", "", 200)
	if st["state"] != "success" {
		t.Fatalf("tn state: %v", st)
	}
	// Policy Segment → Manager Logical Switch(보호 객체), Segment Port → Logical Port
	lss := results(mustNSX(t, e, "GET", "/api/v1/logical-switches", "", 200))
	if len(lss) != 10 || lss[0]["_protection"] != "REQUIRE_OVERRIDE" {
		t.Fatalf("logical switches: %d %v", len(lss), lss[0])
	}
	seg := mustNSX(t, e, "GET", p4+"/infra/segments/seg-01", "", 200)
	ls := mustNSX(t, e, "GET", "/api/v1/logical-switches/"+seg["unique_id"].(string), "", 200)
	if ls["display_name"] != "seg-01" || ls["transport_zone_id"] == "" {
		t.Fatalf("logical switch: %v", ls)
	}
	mustNSX(t, e, "PUT", "/api/v1/logical-switches/"+seg["unique_id"].(string), `{"transport_zone_id":"x","admin_state":"UP","_revision":0}`, 403)
	attachVMToSegment(t, e, "/infra/segments/seg-01")
	deadline := time.Now().Add(5 * time.Second)
	for len(results(mustNSX(t, e, "GET", "/api/v1/logical-ports", "", 200))) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("logical port not derived")
		}
		time.Sleep(25 * time.Millisecond)
	}
	// 새 Segment는 Logical Switch로, 삭제하면 사라진다
	mustNSX(t, e, "PATCH", p4+"/infra/segments/new-ls", `{}`, 200)
	n := mustNSX(t, e, "GET", p4+"/infra/segments/new-ls", "", 200)
	mustNSX(t, e, "GET", "/api/v1/logical-switches/"+n["unique_id"].(string), "", 200)
	mustNSX(t, e, "DELETE", p4+"/infra/segments/new-ls", "", 200)
	mustNSX(t, e, "GET", "/api/v1/logical-switches/"+n["unique_id"].(string), "", 404)
	// 호스트 연결 끊김 → Transport Node 상태 failed
	disconnectHost(t, e, tn["display_name"].(string))
	deadline = time.Now().Add(5 * time.Second)
	for {
		st := mustNSX(t, e, "GET", "/api/v1/transport-nodes/"+tn["id"].(string)+"/state", "", 200)
		if st["state"] == "failed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tn state after disconnect: %v", st)
		}
		time.Sleep(25 * time.Millisecond)
	}
	// Manager POST 컬렉션: 서버가 ID를 정하고 201 + 객체(스펙 응답)
	code, ip := nsxDo(t, e, "POST", "/api/v1/ip-sets", `{"display_name":"ips","ip_addresses":["10.1.1.1"]}`)
	if code != 201 || ip["id"] == "" {
		t.Fatalf("manager create: %d %v", code, ip)
	}
	mustNSX(t, e, "GET", "/api/v1/ip-sets/"+ip["id"].(string), "", 200)
}

// T-TR-COV-0301 T-FR-API-0101 (Phase 4b 응답)
func TestNSXPhase4bResponseSchema(t *testing.T) {
	e := testenv.Start(t, nil)
	dir := e.Sim.Profile.NSXDir
	specs := map[string]*nsxcheck.Spec{}
	for k, f := range map[string]string{"/api/v1": "nsx_api.json", "/policy/api/v1": "nsx_policy_api.json"} {
		s, err := nsxcheck.Load(filepath.Join(dir, f))
		if err != nil {
			t.Skipf("NSX 스펙 없음(Get-NsxSpec.ps1): %v", err)
		}
		specs[k] = s
	}
	attachVMToSegment(t, e, "/infra/segments/seg-01")
	mustNSX(t, e, "PATCH", p4+"/infra/domains/default/security-policies/sp-x", `{"category":"Application","rules":[{"id":"r","action":"ALLOW","source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"]}]}`, 200)
	mustNSX(t, e, "PATCH", p4+"/infra/tier-0s/t0-a/locale-services/default", `{}`, 200)
	tn := results(mustNSX(t, e, "GET", "/api/v1/transport-nodes", "", 200))[0]["id"].(string)
	seg := mustNSX(t, e, "GET", p4+"/infra/segments/seg-01", "", 200)["unique_id"].(string)
	cases := []struct{ method, base, path, template, body string }{
		{"GET", p4, "/infra/domains", "/infra/domains", ""},
		{"GET", p4, "/infra/domains/default/groups", "/infra/domains/{domain-id}/groups", ""},
		{"GET", p4, "/infra/domains/default/groups/grp-01", "/infra/domains/{domain-id}/groups/{group-id}", ""},
		{"GET", p4, "/infra/domains/default/groups/grp-01/members/segments", "/infra/domains/{domain-id}/groups/{group-id}/members/segments", ""},
		{"GET", p4, "/infra/domains/default/groups/grp-01/members/segment-ports", "/infra/domains/{domain-id}/groups/{group-id}/members/segment-ports", ""},
		{"GET", p4, "/infra/domains/default/groups/grp-01/members/ip-addresses", "/infra/domains/{domain-id}/groups/{group-id}/members/ip-addresses", ""},
		{"GET", p4, "/infra/services", "/infra/services", ""},
		{"GET", p4, "/infra/services/HTTPS", "/infra/services/{service-id}", ""},
		{"GET", p4, "/infra/services/HTTPS/service-entries", "/infra/services/{service-id}/service-entries", ""},
		{"GET", p4, "/infra/domains/default/security-policies", "/infra/domains/{domain-id}/security-policies", ""},
		{"GET", p4, "/infra/domains/default/security-policies/sp-x", "/infra/domains/{domain-id}/security-policies/{security-policy-id}", ""},
		{"GET", p4, "/infra/domains/default/security-policies/sp-x/rules", "/infra/domains/{domain-id}/security-policies/{security-policy-id}/rules", ""},
		{"GET", p4, "/infra/domains/default/security-policies/sp-x/rules/r", "/infra/domains/{domain-id}/security-policies/{security-policy-id}/rules/{rule-id}", ""},
		{"PUT", p4, "/infra/domains/default/security-policies/sp-y", "/infra/domains/{domain-id}/security-policies/{security-policy-id}", `{"category":"Application"}`},
		{"POST", p4, "/infra/domains/default/security-policies/sp-x/rules/r?action=revise", "/infra/domains/{domain-id}/security-policies/{security-policy-id}/rules/{rule-id}?action=revise", `{"action":"DROP","source_groups":["ANY"],"destination_groups":["ANY"],"services":["ANY"]}`},
		{"PUT", p4, "/infra/segments/put-seg", "/infra/segments/{segment-id}", `{}`},
		{"GET", p4, "/infra/tier-0s", "/infra/tier-0s", ""},
		{"GET", p4, "/infra/tier-0s/t0-a/locale-services", "/infra/tier-0s/{tier-0-id}/locale-services", ""},
		{"GET", p4, "/infra/sites/default/enforcement-points/default/edge-clusters", "/infra/sites/{site-id}/enforcement-points/{enforcementpoint-id}/edge-clusters", ""},
		{"GET", p4, "/search/query?query=resource_type:Segment", "/search/query", ""},
		{"GET", "/api/v1", "/search/query?query=resource_type:TransportNode", "/search/query", ""},
		{"GET", "/api/v1", "/transport-nodes", "/transport-nodes", ""},
		{"GET", "/api/v1", "/transport-nodes/" + tn, "/transport-nodes/{transport-node-id}", ""},
		{"GET", "/api/v1", "/transport-nodes/" + tn + "/state", "/transport-nodes/{transport-node-id}/state", ""},
		{"GET", "/api/v1", "/logical-switches", "/logical-switches", ""},
		{"GET", "/api/v1", "/logical-switches/" + seg, "/logical-switches/{lswitch-id}", ""},
		{"GET", "/api/v1", "/logical-ports", "/logical-ports", ""},
		{"GET", "/api/v1", "/edge-clusters", "/edge-clusters", ""},
		{"POST", "/api/v1", "/ip-sets", "/ip-sets", `{"ip_addresses":["10.0.0.1"]}`},
	}
	var failures []string
	for _, c := range cases {
		code, b := nsxRaw(t, e, c.method, c.base+c.path, c.body)
		if code/100 != 2 {
			failures = append(failures, fmt.Sprintf("%s %s: status %d (%s)", c.method, c.path, code, b))
			continue
		}
		if errs := specs[c.base].Check(c.method, c.template, code, b); len(errs) > 0 {
			sort.Strings(errs)
			if len(errs) > 8 {
				errs = append(errs[:8], "...")
			}
			failures = append(failures, fmt.Sprintf("%s %s%s (%d): %s", c.method, c.base, c.path, code, strings.Join(errs, "; ")))
		}
	}
	if len(failures) > 0 {
		t.Fatal(strings.Join(failures, "\n"))
	}
	_ = http.MethodGet
}
