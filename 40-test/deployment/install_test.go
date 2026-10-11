// macOS·Linux 설치 자동화(WP 5.10, NFR-ARC-0308) 시험. 시스템을 바꾸지 않도록 임시 hosts 파일과 --dry-run만 쓴다.
// 실제 관리자 권한 단계(/etc/hosts, 루프백 별칭, 키체인·CA 저장소, setcap)는 사용자가 실행한다(실행 확인 대기).
package deployment

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

func script(t *testing.T) (string, string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash 없음")
	}
	return bash, filepath.Join(testenv.Repo(), "50-deployment", "install", "install-cloudinfra-sim.sh")
}

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	bash, sh := script(t)
	out, err := exec.Command(bash, append([]string{sh}, args...)...).CombinedOutput()
	return string(out), err
}

// T-NFR-ARC-0308: hosts 등록 멱등·제거, OS별 단계(dry-run), 예약 TLD 검사
func TestInstallScript(t *testing.T) {
	hosts := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(hosts, []byte("127.0.0.1 localhost\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // 두 번 실행해도 항목은 한 번만
		if out, err := run(t, "--os", "linux", "--hosts-file", hosts, "--skip-ca", "--esx-hosts", "2"); err != nil {
			t.Fatalf("install: %v %s", err, out)
		}
	}
	b, _ := os.ReadFile(hosts)
	want := "127.0.0.1 localhost\n127.0.0.2 vcenter.cloudinfra-sim.test # cloudinfra-sim\n127.0.0.3 nsx.cloudinfra-sim.test # cloudinfra-sim\n" +
		"127.0.0.2 esx-01.cloudinfra-sim.test # cloudinfra-sim\n127.0.0.2 esx-02.cloudinfra-sim.test # cloudinfra-sim\n"
	if string(b) != want {
		t.Fatalf("hosts:\n%s", b)
	}
	if out, err := run(t, "--os", "linux", "--hosts-file", hosts, "--skip-ca", "--uninstall"); err != nil {
		t.Fatalf("uninstall: %v %s", err, out)
	}
	if b, _ := os.ReadFile(hosts); string(b) != "127.0.0.1 localhost\n" {
		t.Fatalf("uninstall left entries:\n%s", b)
	}
	// macOS: 루프백 별칭·LaunchDaemon·키체인
	out, err := run(t, "--os", "darwin", "--dry-run", "--hosts-file", hosts, "--persist", "--system", "--data-dir", "/opt/sim")
	if err != nil {
		t.Fatalf("darwin dry-run: %v %s", err, out)
	}
	for _, w := range []string{"ifconfig lo0 alias 127.0.0.2 up", "ifconfig lo0 alias 127.0.0.3 up", "launchctl load -w /Library/LaunchDaemons/test.cloudinfra-sim.loopback.plist",
		"security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain /opt/sim/certs/ca.pem"} {
		if !strings.Contains(out, w) {
			t.Errorf("darwin dry-run missing %q:\n%s", w, out)
		}
	}
	// Linux: 별칭 없음, 시스템 CA 저장소, 443 권한
	out, err = run(t, "--os", "linux", "--dry-run", "--hosts-file", hosts, "--data-dir", "/opt/sim", "--binary", "/opt/sim/cloudinfra-sim")
	if err != nil {
		t.Fatalf("linux dry-run: %v %s", err, out)
	}
	if strings.Contains(out, "ifconfig") || !strings.Contains(out, "/opt/sim/certs/ca.pem") || !strings.Contains(out, "setcap cap_net_bind_service=+ep /opt/sim/cloudinfra-sim") {
		t.Errorf("linux dry-run:\n%s", out)
	}
	if out, err := run(t, "--os", "linux", "--domain", "example.com", "--dry-run"); err == nil || !strings.Contains(out, "NFR-0314") {
		t.Errorf("non-reserved TLD must be refused: %v %s", err, out)
	}
}
