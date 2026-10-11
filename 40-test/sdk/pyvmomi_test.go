package sdk

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

// pyVmomi(9.1.1.0, D:\AISCP\tools\specs\pyvmomi-9.1.1.0)로 연결·버전 협상·인벤토리·VM 생명주기·오류를 확인한다.
// 시험 본문은 sdk/pyvmomi/test_pyvmomi.py(unittest)다. PYTHON(기본 Windows "py -3", 그 외 python3)·PYVMOMI_DIR로 바꾼다.
// 추적: T-TR-COV-0305 T-NFR-ARC-0109 T-NFR-ARC-0113 T-FR-VC-0605
func TestPyVmomi(t *testing.T) {
	dir := tool(t, "PYVMOMI_DIR", `D:\AISCP\tools\specs\pyvmomi-9.1.1.0`)
	py, args := os.Getenv("PYTHON"), []string{}
	if py == "" {
		py = "python3"
		if runtime.GOOS == "windows" {
			py, args = "py", []string{"-3"}
		}
	}
	if _, err := exec.LookPath(py); err != nil {
		t.Skipf("Python 없음: %v", err)
	}
	e := testenv.Start(t, nil)
	hp, tp := endpoint(t, e)
	host, port, _ := strings.Cut(hp, ":")
	u := e.User("vcenter")
	script, _ := filepath.Abs(filepath.Join("pyvmomi", "test_pyvmomi.py"))
	cmd := exec.Command(py, append(args, "-I", script)...) // -I: 현재 디렉터리·환경 경로를 모듈 경로에 넣지 않음
	cmd.Env = append(os.Environ(), "PYVMOMI_DIR="+dir, "SIM_HOST="+host, "SIM_PORT="+port,
		"SIM_USER="+u.Name, "SIM_PASSWORD="+u.Password, "SIM_THUMBPRINT="+tp, "PYTHONIOENCODING=utf-8",
		"SIM_ABOUT_VERSION="+e.Sim.Profile.VCenter.About["version"], "SIM_ABOUT_BUILD="+e.Sim.Profile.VCenter.About["build"],
		"SIM_ABOUT_API="+e.Sim.Profile.VCenter.APIVersion)
	out, err := cmd.CombinedOutput()
	t.Logf("pyVmomi:\n%s", out)
	if err != nil {
		t.Fatalf("pyVmomi tests failed: %v", err)
	}
}
