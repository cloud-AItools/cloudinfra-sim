// vCenter 버전 계약 테스트 (FR-VC-0207, FR-VC-0605 ~ FR-VC-0607, AC-16의 MVP 범위).
// 기대값 근거: SRC-VIM-TYPEINFO(pyVmomi 도입 버전). BEHAVIOR_SPEC §6의 MethodNotFound는 가정이다.
package contract

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"

	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-VC-0207: 버전 협상 문서는 프로파일 버전을 노출한다.
func TestVimServiceVersions(t *testing.T) {
	e := testenv.Start(t, nil)
	res, err := e.Client.Get("https://vcenter." + testenv.Domain + "/sdk/vimServiceVersions.xml")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	s := string(b)
	api := e.Sim.Profile.VCenter.APIVersion
	if res.StatusCode != 200 || !strings.Contains(s, "<version>"+api+"</version>") || !strings.Contains(s, "<version>6.7.3</version>") {
		t.Fatalf("vimServiceVersions: %d %s", res.StatusCode, s)
	}
	// 프로파일 버전보다 높은 버전을 알리지 않는다
	for _, v := range regexp.MustCompile(`<version>([^<]+)</version>`).FindAllStringSubmatch(s, -1) {
		if profile.Newer(v[1], api) {
			t.Fatalf("%s profile must not advertise %s: %s", api, v[1], s)
		}
	}
}

// T-FR-VC-0606 / AC-16(MVP): 7.0.3 프로파일에서 6.7.3으로 협상한 세션에는 7.0 도입 필드가 없다.
// 추적: T-FR-VC-0604 T-NFR-ARC-0105 T-FR-VC-0605 T-TR-COV-0301
// 추적: T-FR-VC-0307
func TestVersionAwareSerialization(t *testing.T) {
	e := testenv.Start(t, nil)
	// 양성 대조군: 7.0.0.0 도입 필드(backingType, segmentId)와 8.0.0.1 도입 필드(rebootPowerOff)를 채운다.
	setControlFields(t, e)

	for _, tc := range []struct {
		session            string
		wantBacking, wantR bool
	}{
		{"7.0.3.0", true, false}, // 7.0 필드 유지, 8.x 필드 제거
		{"8.0.3.0", true, false}, // 프로파일보다 높은 요청 → 7.0.3으로 하향
		{"6.7.3", false, false},  // 7.0 필드도 제거
	} {
		t.Run(tc.session, func(t *testing.T) {
			c := e.Login(tc.session)
			ctx := context.Background()
			pgRef := firstRef(t, e, "DistributedVirtualPortgroup")
			vmRef := firstRef(t, e, "VirtualMachine")
			var pg mo.DistributedVirtualPortgroup
			if err := c.RetrieveOne(ctx, pgRef, []string{"config"}, &pg); err != nil {
				t.Fatal(err)
			}
			var vm mo.VirtualMachine
			if err := c.RetrieveOne(ctx, vmRef, []string{"config"}, &vm); err != nil {
				t.Fatal(err)
			}
			gotBacking := pg.Config.BackingType != ""
			gotR := vm.Config.RebootPowerOff != nil
			if gotBacking != tc.wantBacking || gotR != tc.wantR {
				t.Fatalf("session %s: backingType present=%v (want %v), rebootPowerOff present=%v (want %v)",
					tc.session, gotBacking, tc.wantBacking, gotR, tc.wantR)
			}
			// 속성 경로로 직접 요청해도 같은 규칙(propSet의 name이 경로, R-11)
			var pg2 mo.DistributedVirtualPortgroup
			if err := c.RetrieveOne(ctx, pgRef, []string{"config.backingType", "config.name"}, &pg2); err != nil {
				t.Fatal(err)
			}
			if (pg2.Config.BackingType != "") != tc.wantBacking || pg2.Config.Name == "" {
				t.Fatalf("session %s: path config.backingType present=%v (want %v), config.name=%q",
					tc.session, pg2.Config.BackingType != "", tc.wantBacking, pg2.Config.Name)
			}
			// WaitForUpdatesEx의 changeSet도 경로 기준으로 거른다
			pc, err := property.DefaultCollector(c.Client).Create(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer pc.Destroy(ctx)
			var gotPath bool
			wctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			_ = property.Wait(wctx, pc, pgRef, []string{"config.backingType", "name"}, func(changes []types.PropertyChange) bool {
				for _, ch := range changes {
					if ch.Name == "config.backingType" {
						gotPath = true
					}
				}
				return true
			})
			if gotPath != tc.wantBacking {
				t.Fatalf("session %s: changeSet config.backingType present=%v (want %v)", tc.session, gotPath, tc.wantBacking)
			}
		})
	}
}

// T-FR-VC-0607: 세션 버전에 없는 메서드는 거부한다(6.0 세션에서 6.5 도입 메서드).
// 추적: T-FR-VC-0603
func TestMethodNotInSessionVersion(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	for _, tc := range []struct {
		session string
		wantErr bool
	}{{"6.0", true}, {"6.5", false}} {
		c := e.Login(tc.session)
		root := c.ServiceContent.RootFolder
		_, err := methods.FetchUserPrivilegeOnEntities(ctx, c.Client, &types.FetchUserPrivilegeOnEntities{
			This: *c.ServiceContent.AuthorizationManager, Entities: []types.ManagedObjectReference{root}, UserName: e.User("vcenter").Name,
		})
		if tc.wantErr {
			if err == nil || !soap.IsSoapFault(err) {
				t.Fatalf("session %s: want fault, got %v", tc.session, err)
			}
			if _, ok := soap.ToSoapFault(err).VimFault().(types.MethodNotFound); !ok {
				t.Fatalf("session %s: want MethodNotFound, got %T (%v)", tc.session, soap.ToSoapFault(err).VimFault(), err)
			}
		} else if err != nil {
			t.Fatalf("session %s: unexpected error %v", tc.session, err)
		}
	}
}

func firstRef(t *testing.T, e *testenv.Env, kind string) types.ManagedObjectReference {
	t.Helper()
	objs := e.Sim.VC.Model().Map().AllReference(kind)
	if len(objs) == 0 {
		t.Fatalf("no %s in fixture", kind)
	}
	return objs[0].Reference()
}

// T-FR-VC-0606 보조: 필터 단위 동작(필드 제거 수)도 확인한다.
func TestFilterUnit(t *testing.T) {
	meta, err := profile.LoadVersionMeta(testenv.Repo() + "/30-implementation/data/compatibility-profiles/vcenter/7.0.3.02500/vim-typeinfo.json")
	if err != nil {
		t.Fatal(err)
	}
	in := `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><soapenv:Body>` +
		`<RetrievePropertiesExResponse xmlns="urn:vim25"><returnval><objects><obj type="DistributedVirtualPortgroup">dvportgroup-1</obj>` +
		`<propSet><name>config</name><val xsi:type="DVPortgroupConfigInfo"><key>dvportgroup-1</key><name>pg</name><numPorts>8</numPorts><type>earlyBinding</type>` +
		`<backingType>nsx</backingType><segmentId>/infra/segments/web</segmentId></val></propSet></objects></returnval></RetrievePropertiesExResponse></soapenv:Body></soapenv:Envelope>`
	out, st := meta.FilterSOAP([]byte(in), "6.7.3")
	if st.Fields != 2 || strings.Contains(string(out), "backingType") || !strings.Contains(string(out), "<numPorts>8</numPorts>") {
		t.Fatalf("filter: %+v %s", st, out)
	}
	if _, st := meta.FilterSOAP([]byte(in), "7.0.3.0"); st.Fields != 0 {
		t.Fatalf("7.0.3 session must keep 7.0 fields: %+v", st)
	}
}

// setControlFields는 vcsim 객체에 버전 대조군 값을 직접 넣는다(vcsim 기본 데이터에는 거의 없음).
func setControlFields(t *testing.T, e *testenv.Env) {
	t.Helper()
	m := e.Sim.VC.Model()
	for _, obj := range m.Map().AllReference("DistributedVirtualPortgroup") {
		pg := obj.(*simulator.DistributedVirtualPortgroup)
		pg.Config.BackingType = "nsx"
		pg.Config.SegmentId = "/infra/segments/web"
	}
	for _, obj := range m.Map().AllReference("VirtualMachine") {
		obj.(*simulator.VirtualMachine).Config.RebootPowerOff = types.NewBool(true)
	}
}
