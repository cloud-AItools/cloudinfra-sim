// Package ansible은 Ansible 자체 테스트(TR-COV-0306, 2026-10-11 사용자 결정: Ansible만 채택, Terraform 미채택)를 돌린다.
// 시뮬레이터를 띄우고 simulator.yml(community.vmware + ansible.builtin.uri)을 ansible-playbook으로 실행한다.
//
// 제어 노드: Linux·macOS·WSL(Ansible은 Windows를 제어 노드로 지원하지 않는다). 준비:
//
//	python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
//	.venv/bin/ansible-galaxy collection install -r requirements.yml
//	PATH=$PWD/.venv/bin:$PATH go test ./ansible -run TestAnsiblePlaybook -v
//
// ansible-playbook이 없으면 건너뛴다. ANSIBLE_REQUIRED=1(CI)이면 건너뛰지 않고 실패한다.
// ANSIBLE_PLAYBOOK으로 실행파일 경로를 지정할 수 있다.
package ansible

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"cloudinfra-sim/test/internal/testenv"
)

func playbookBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("ANSIBLE_PLAYBOOK")
	if bin == "" {
		if p, err := exec.LookPath("ansible-playbook"); err == nil {
			bin = p
		}
	}
	if bin == "" {
		msg := "ansible-playbook이 없다(제어 노드: Linux·macOS·WSL, 준비는 패키지 설명 참조)"
		if os.Getenv("ANSIBLE_REQUIRED") == "1" {
			t.Fatal(msg)
		}
		t.Skip(msg)
	}
	return bin
}

// resolvesTo는 name이 ip로 해석되는지다(hosts 등록 여부 — 이름 접속이면 인증서를 검증할 수 있다).
func resolvesTo(name, ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, name)
	return err == nil && slices.Contains(addrs, ip)
}

var recap = regexp.MustCompile(`(?m)^localhost\s+:\s+ok=(\d+)\s+changed=(\d+)\s+unreachable=(\d+)\s+failed=(\d+)`)

// T-TR-COV-0306 T-TR-COV-0305: 범용 자동화 도구(Ansible)로 vCenter 조회·VM 생명주기·멱등성·인증 실패, NSX Segment CRUD를 수행한다.
func TestAnsiblePlaybook(t *testing.T) {
	bin := playbookBin(t)
	if out, err := exec.Command(bin, "--version").CombinedOutput(); err == nil {
		t.Logf("ansible:\n%s", firstLines(string(out), 3))
	}
	if gal, err := exec.LookPath(filepath.Join(filepath.Dir(bin), "ansible-galaxy")); err == nil {
		if out, err := exec.Command(gal, "collection", "list", "community.vmware").CombinedOutput(); err == nil {
			t.Logf("collections:\n%s", strings.TrimSpace(string(out)))
		}
	}

	e := testenv.Start(t, nil)
	vcHost, vcPort, _ := net.SplitHostPort(e.Sim.Addrs["vcenter"].String())
	nsxHost, nsxPort, _ := net.SplitHostPort(e.Sim.Addrs["nsx"].String())
	vcName, nsxName := "vcenter."+testenv.Domain, "nsx."+testenv.Domain
	validate := resolvesTo(vcName, vcHost) && resolvesTo(nsxName, nsxHost)
	if validate {
		vcHost, nsxHost = vcName, nsxName
	} else {
		t.Logf("%s·%s가 hosts에 없어 IP로 접속하고 인증서 검증을 끈다(인증서는 이름만 담는다)", vcName, nsxName)
	}
	ca := filepath.Join(e.Sim.Cfg.DataDir, "certs", "ca.pem")
	if _, err := os.Stat(ca); err != nil {
		t.Fatalf("시뮬레이터 CA: %v", err)
	}
	vu, nu := e.User("vcenter"), e.User("nsx")

	cmd := exec.Command(bin, "-i", "localhost,", "-e", "ansible_python_interpreter={{ ansible_playbook_python }}", "simulator.yml")
	cmd.Env = append(os.Environ(),
		"SIM_VC_HOST="+vcHost, "SIM_VC_PORT="+vcPort,
		"SIM_NSX_URL=https://"+net.JoinHostPort(nsxHost, nsxPort),
		"SIM_VC_USER="+vu.Name, "SIM_VC_PASSWORD="+vu.Password, "SIM_NSX_USER="+nu.Name, "SIM_NSX_PASSWORD="+nu.Password,
		"SIM_VALIDATE_CERTS="+strconv.FormatBool(validate), "SIM_CA_FILE="+ca,
		"SSL_CERT_FILE="+ca, // pyVmomi(ssl 기본 컨텍스트)가 시뮬레이터 CA를 신뢰
		"ANSIBLE_NOCOLOR=1", "ANSIBLE_RETRY_FILES_ENABLED=0", "ANSIBLE_LOCALHOST_WARNING=0",
	)
	start := time.Now()
	out, err := cmd.CombinedOutput()
	text := string(out)
	m := recap.FindStringSubmatch(text)
	if err != nil || m == nil || m[3] != "0" || m[4] != "0" {
		t.Fatalf("ansible-playbook 실패(%v, %s):\n%s", err, time.Since(start).Round(time.Millisecond), tail(text, 6000))
	}
	t.Logf("PLAY RECAP ok=%s changed=%s failed=0 (%s, 인증서 검증 %v)", m[1], m[2], time.Since(start).Round(time.Millisecond), validate)
}

func firstLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	return strings.Join(l[:min(n, len(l))], "\n")
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
