// 범용 SDK 통합 시험 (PRD-2 TR-COV-0305, NFR-ARC-0109): govc·pyVmomi로 시뮬레이터를 호출한다.
// 이 SDK들은 vcsim과 같은 계열이거나 관대한 편이므로, 통과가 다른 SDK의 통과를 보장하지 않는다(TR-COV-0305).
// 도구 위치: GOVC(기본 D:\AISCP\tools\govc\govc.exe, 40-test/tools/govc 모듈로 빌드), PYVMOMI_DIR·PYTHON.
// 도구가 없으면 Skip한다(증거 생성기는 Skip을 통과로 세지 않는다).
package sdk

import (
	"crypto/x509"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/soap"

	"cloudinfra-sim/test/internal/testenv"
)

func tool(t *testing.T, env, def string) string {
	t.Helper()
	p := os.Getenv(env)
	if p == "" {
		p = def
	}
	if _, err := os.Stat(p); err != nil {
		if lp, err := exec.LookPath(p); err == nil {
			return lp
		}
		t.Skipf("%s 없음(%s): %v", env, p, err)
	}
	return p
}

// endpoint는 vCenter 리스너의 IP 주소와 인증서 SHA-256 지문이다(이름 없이 IP로 접속, hosts 등록 불필요).
func endpoint(t *testing.T, e *testenv.Env) (hostport, thumbprint string) {
	t.Helper()
	cert, err := x509.ParseCertificate(e.Sim.Certs.Server.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return e.Sim.Addrs["vcenter"].String(), soap.ThumbprintSHA256(cert)
}

type govc struct {
	t   *testing.T
	bin string
	env []string
}

func newGovc(t *testing.T, e *testenv.Env, vimVersion string) *govc {
	t.Helper()
	bin := tool(t, "GOVC", `D:\AISCP\tools\govc\govc.exe`)
	hp, tp := endpoint(t, e)
	kh := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(kh, []byte(hp+" "+tp+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	u := e.User("vcenter")
	env := append(os.Environ(),
		"GOVC_URL=https://"+hp+"/sdk", "GOVC_USERNAME="+u.Name, "GOVC_PASSWORD="+u.Password,
		"GOVC_TLS_KNOWN_HOSTS="+kh, "GOVC_INSECURE=false", "GOVC_PERSIST_SESSION=false")
	if vimVersion != "" {
		env = append(env, "GOVC_VIM_VERSION="+vimVersion)
	}
	return &govc{t: t, bin: bin, env: env}
}

func (g *govc) run(args ...string) (string, error) {
	g.t.Helper()
	cmd := exec.Command(g.bin, args...)
	cmd.Env = g.env
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (g *govc) must(args ...string) string {
	g.t.Helper()
	out, err := g.run(args...)
	if err != nil {
		g.t.Fatalf("govc %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// govc(govmomi v0.56.0 태그 커밋으로 빌드)로 연결·조회·VM 생명주기·오류를 확인한다.
// 추적: T-TR-COV-0305 T-NFR-ARC-0109 T-TR-COV-0302
func TestGovcLifecycle(t *testing.T) {
	e := testenv.Start(t, nil)
	g := newGovc(t, e, "")

	// 연결·버전(govc는 기본으로 최신 vim 버전을 요청하고, 세션은 프로파일 api_version으로 하향된다)
	var about struct {
		About struct {
			Version, Build, ApiVersion string
		}
	}
	if err := json.Unmarshal([]byte(g.must("about", "-json")), &about); err != nil {
		t.Fatal(err)
	}
	a := e.Sim.Profile.VCenter.About
	if about.About.Version != a["version"] || about.About.Build != a["build"] || about.About.ApiVersion != a["apiVersion"] {
		t.Fatalf("about: %+v", about.About)
	}
	// 인벤토리
	vms := strings.Fields(g.must("find", "/", "-type", "m"))
	if len(vms) != 20 {
		t.Fatalf("VMs %d, want 20 (default-small)", len(vms))
	}
	ds := strings.Fields(g.must("find", "/", "-type", "s"))
	if len(ds) == 0 {
		t.Fatal("no datastore")
	}
	dsName := filepath.Base(ds[0])
	nets := strings.Fields(g.must("find", "/", "-type", "DistributedVirtualPortgroup"))
	if len(nets) == 0 {
		t.Fatal("no portgroup")
	}
	net := filepath.Base(nets[0])

	// 생성 → 조회 → 변경 → 전원 → 삭제
	g.must("vm.create", "-on=false", "-c", "1", "-m", "512", "-ds", dsName, "-net", net, "sdk-govc")
	g.must("vm.change", "-vm", "sdk-govc", "-c", "2", "-m", "1024")
	var info struct {
		VirtualMachines []struct {
			Config struct {
				Hardware struct {
					NumCPU   int32
					MemoryMB int32
				}
			}
			Runtime struct{ PowerState string }
		}
	}
	if err := json.Unmarshal([]byte(g.must("vm.info", "-json", "sdk-govc")), &info); err != nil {
		t.Fatal(err)
	}
	if len(info.VirtualMachines) != 1 || info.VirtualMachines[0].Config.Hardware.NumCPU != 2 || info.VirtualMachines[0].Config.Hardware.MemoryMB != 1024 {
		t.Fatalf("vm.info after change: %+v", info)
	}
	g.must("vm.power", "-on", "sdk-govc")
	// 전원이 켜진 VM의 CPU 감소는 거부(InvalidPowerState, behavior-rules)
	if out, err := g.run("vm.change", "-vm", "sdk-govc", "-c", "1"); err == nil || !strings.Contains(out, "powered") && !strings.Contains(out, "InvalidPowerState") {
		t.Fatalf("decrease CPU while powered on must fail: %v %s", err, out)
	}
	// 중복 이름 생성은 DuplicateName
	if out, err := g.run("vm.create", "-on=false", "-ds", dsName, "-net", net, "sdk-govc"); err == nil || !strings.Contains(out, "already exists") && !strings.Contains(out, "DuplicateName") {
		t.Fatalf("duplicate create must fail: %v %s", err, out)
	}
	g.must("vm.power", "-off", "sdk-govc")
	g.must("vm.destroy", "sdk-govc")
	if out, _ := g.run("find", "/", "-type", "m", "-name", "sdk-govc"); out != "" {
		t.Fatalf("VM remains after destroy: %s", out)
	}
	// 잘못된 Credential
	bad := newGovc(t, e, "")
	for i, kv := range bad.env {
		if strings.HasPrefix(kv, "GOVC_PASSWORD=") {
			bad.env[i] = "GOVC_PASSWORD=wrong"
		}
	}
	// faultstring은 vcsim 문구("Login failure")다. 실제 vCenter 문구와 다를 수 있다(Fault 형식은 Scenario 01에서 InvalidLogin 확인, R-26).
	if out, err := bad.run("about"); err == nil || !strings.Contains(out, "Login failure") {
		t.Fatalf("invalid login must fail: %v %s", err, out)
	}
}

// govc로 협상 버전을 낮춘 세션(6.7.3)에서 7.0 도입 속성이 빠지는지 확인한다(AC-16 MVP 범위, 다른 SDK 경로).
// 추적: T-FR-VC-0605 T-FR-VC-0606 T-TR-COV-0305
func TestGovcVersionDowngrade(t *testing.T) {
	e := testenv.Start(t, nil)
	// 7.0 도입 필드의 양성 대조군(Fixture는 DVPG backingType을 채우지 않음, D8 연결은 Phase 3)
	for _, obj := range e.Sim.VC.Model().Map().AllReference("DistributedVirtualPortgroup") {
		obj.(*simulator.DistributedVirtualPortgroup).Config.BackingType = "standard"
	}
	backing := func(version string) string {
		g := newGovc(t, e, version)
		pgs := strings.Fields(g.must("find", "/", "-type", "DistributedVirtualPortgroup"))
		if len(pgs) == 0 {
			t.Fatal("no DVPG")
		}
		return g.must("object.collect", "-s", pgs[0], "config.backingType")
	}
	if v := backing("7.0.3.0"); v == "" {
		t.Fatal("7.0.3 session must return config.backingType")
	}
	if v := backing("6.7.3"); v != "" {
		t.Fatalf("6.7.3 session must not return config.backingType (7.0 field): %q", v)
	}
}
