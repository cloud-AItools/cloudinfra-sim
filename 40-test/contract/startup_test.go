// 기동·운영 계약 테스트: AC-15(FR-API-0301·0302), NFR-0315, NFR-0305, TR-SCN-0102, FR-BE-0210, FR-DM-0204.
package contract

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vmware/govmomi/find"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/internal/sim"
	"cloudinfra-sim/test/internal/testenv"
)

func newSim(t *testing.T, mutate func(*config.Config)) error {
	t.Helper()
	impl := filepath.Join(testenv.Repo(), "30-implementation")
	cfg, err := config.Parse([]string{
		"--vcenter-version", "7.0.3.02500", "--nsx-version", "3.2.4.3",
		"--data-dir", t.TempDir(),
		"--profiles-dir", filepath.Join(impl, "data", "compatibility-profiles"),
		"--fixture", filepath.Join(impl, "data", "fixtures", "default-small.yaml"),
		"--verification-mode", "development",
	}, func(string) string { return "" })
	if err != nil {
		return err
	}
	if mutate != nil {
		mutate(cfg)
	}
	s, err := sim.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		s.VC.Close()
	}
	return err
}

// T-FR-API-0302 / AC-15: 미등록 버전, 허용되지 않은 조합, 스키마 오류, 검증 모드별 기준 미달은 기동을 거부한다.
// 추적: T-FR-API-0305 T-FR-VC-0601
// 추적: T-FR-VC-0602
func TestStartupRefusals(t *testing.T) {
	impl := filepath.Join(testenv.Repo(), "30-implementation", "data", "compatibility-profiles")
	broken := t.TempDir()
	copyTree(t, impl, broken, func(rel string) bool {
		return !strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, "vim-typeinfo.json")
	})
	// 스키마 오류 프로파일: execution_scope를 비운다.
	p := filepath.Join(broken, "vcenter", "7.0.3.02500", "profile.yaml")
	b, _ := os.ReadFile(p)
	_ = os.WriteFile(p, []byte(strings.Replace(string(b), "execution_scope: [", "execution_scope: []\n#", 1)), 0o644)

	cases := []struct {
		name   string
		mutate func(*config.Config)
		want   string
	}{
		{"unregistered vCenter", func(c *config.Config) { c.VCenterVersion = "8.0.3.00000" }, "7.0.3.02500)"},
		{"unregistered NSX", func(c *config.Config) { c.NSXVersion = "4.2.0" }, "NSX 프로파일 4.2.0"},
		{"schema error", func(c *config.Config) { c.ProfilesDir = broken }, "execution_scope는 비울 수 없다"},
		{"release without evidence", func(c *config.Config) { c.VerificationMode = "release" }, "release 모드 기동 조건 미달"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := newSim(t, tc.mutate)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want refusal containing %q, got %v", tc.want, err)
			}
			t.Logf("refused: %s", strings.SplitN(err.Error(), "\n", 3)[0])
		})
	}
	if err := newSim(t, nil); err != nil {
		t.Fatalf("development mode with Contract Defined scope must start: %v", err)
	}
	// 허용되지 않은 조합: matrix에 없는 조합을 만든다.
	other := t.TempDir()
	copyTree(t, impl, other, func(rel string) bool {
		return !strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, "vim-typeinfo.json")
	})
	_ = os.WriteFile(filepath.Join(other, "compatibility-matrix.yaml"), []byte("combinations:\n  - {vcenter: 7.0.3.02500, nsx: 3.2.1}\n"), 0o644)
	err := newSim(t, func(c *config.Config) { c.ProfilesDir = other })
	if err == nil || !strings.Contains(err.Error(), "허용되지 않은 조합") {
		t.Fatalf("disallowed combination: %v", err)
	}
}

func copyTree(t *testing.T, src, dst string, keep func(string) bool) {
	t.Helper()
	_ = filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !keep(filepath.ToSlash(rel)) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

// T-NFR-0315: 인증서 지문은 Reset과 재기동(같은 데이터 디렉터리) 후에도 유지된다.
func TestCertificateThumbprintStable(t *testing.T) {
	dir := t.TempDir()
	e1 := testenv.Start(t, func(c *config.Config) { c.DataDir = dir })
	first := e1.Sim.Certs.ThumbprintSHA256()
	res, err := e1.AdminDo(http.MethodPost, "/v1/reset", nil)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("reset: %v %v", err, res)
	}
	res.Body.Close()
	if e1.Sim.Certs.ThumbprintSHA256() != first {
		t.Fatal("thumbprint changed after reset")
	}
	e1.Sim.Shutdown(context.Background())
	e2 := testenv.Start(t, func(c *config.Config) { c.DataDir = dir })
	if got := e2.Sim.Certs.ThumbprintSHA256(); got != first {
		t.Fatalf("thumbprint changed after restart: %s != %s", got, first)
	}
}

// T-NFR-0305: Admin API는 토큰 없이 거부하고, 루프백 외 주소에는 바인딩하지 않는다(NFR-ARC-0305).
// 추적: T-FR-API-0304 T-NFR-ARC-0305
func TestAdminAPIAuth(t *testing.T) {
	e := testenv.Start(t, nil)
	res, err := http.Get("http://" + e.Sim.Addrs["admin"].String() + "/v1/status")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("admin without token: %d", res.StatusCode)
	}
	res, _ = e.AdminDo(http.MethodGet, "/v1/status", nil)
	var st map[string]any
	_ = json.NewDecoder(res.Body).Decode(&st)
	res.Body.Close()
	if st["verification_mode"] != "development" {
		t.Fatalf("status: %v", st)
	}
	// 루프백이 아닌 Admin 주소는 Start에서 거부한다.
	impl := filepath.Join(testenv.Repo(), "30-implementation")
	cfg, _ := config.Parse([]string{
		"--vcenter-version", "7.0.3.02500", "--nsx-version", "3.2.4.3", "--data-dir", t.TempDir(),
		"--vcenter-addr", "127.0.0.2:0", "--nsx-addr", "127.0.0.3:0", "--admin-addr", "0.0.0.0:0",
		"--profiles-dir", filepath.Join(impl, "data", "compatibility-profiles"),
		"--fixture", filepath.Join(impl, "data", "fixtures", "default-small.yaml"),
	}, func(string) string { return "" })
	s, err := sim.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(context.Background())
	if err := s.Start(); err == nil || !strings.Contains(err.Error(), "루프백") {
		t.Fatalf("non-loopback admin address must be refused at Start: %v", err)
	}
}

// T-TR-SCN-0102 / T-FR-BE-0210: Reset 후 Fixture 상태로 돌아가고, 같은 Seed는 같은 ID를 만든다.
// 추적: T-TR-SCN-0101 T-TR-SCN-0103 T-TR-SCN-0202 T-NFR-0108 T-FR-DM-0210 T-NFR-0203
func TestResetDeterminism(t *testing.T) {
	e := testenv.Start(t, nil)
	ctx := context.Background()
	ids := func() []string {
		c := e.Login("")
		vms, err := find.NewFinder(c.Client, true).VirtualMachineList(ctx, "/*/vm/*")
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, vm := range vms {
			out = append(out, vm.Reference().Value)
		}
		return out
	}
	before := ids()
	res, _ := e.AdminDo(http.MethodPost, "/v1/reset", nil)
	res.Body.Close()
	after := ids()
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatalf("VM MoRefs differ after reset:\n%v\n%v", before, after)
	}
	t.Logf("same %d VM MoRefs after reset (e.g. %s)", len(after), after[0])
}

// T-FR-DM-0204 / T-FR-VC-0205(범위 밖·미검증): REST VM ID는 SOAP MoRef 값과 같다.
// 추적: T-FR-API-0202 T-FR-VC-0205
func TestRESTViewMatchesSOAP(t *testing.T) {
	e := testenv.Start(t, nil)
	u := e.User("vcenter")
	req, _ := http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(u.Name, u.Password)
	res, err := e.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var token string
	_ = json.NewDecoder(res.Body).Decode(&token)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated || token == "" {
		t.Fatalf("POST /api/session: %d", res.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.SetBasicAuth(u.Name, "wrong")
	res, _ = e.Client.Do(req)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password REST session: %d", res.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodPost, "https://vcenter."+testenv.Domain+"/api/session", nil)
	req.Header.Set("Authorization", "SIGN token=forged")
	res, _ = e.Client.Do(req)
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged SIGN token must be rejected (D10): %d", res.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, "https://vcenter."+testenv.Domain+"/api/vcenter/vm", nil)
	req.Header.Set("vmware-api-session-id", token)
	res, _ = e.Client.Do(req)
	var vms []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&vms)
	res.Body.Close()
	soapIDs := map[string]bool{}
	for _, o := range e.Sim.VC.Model().Map().AllReference("VirtualMachine") {
		soapIDs[o.Reference().Value] = true
	}
	if len(vms) != len(soapIDs) {
		t.Fatalf("REST %d VMs, SOAP %d", len(vms), len(soapIDs))
	}
	for _, vm := range vms {
		if !soapIDs[vm["vm"].(string)] {
			t.Fatalf("REST id %v is not a SOAP MoRef", vm["vm"])
		}
	}
}
