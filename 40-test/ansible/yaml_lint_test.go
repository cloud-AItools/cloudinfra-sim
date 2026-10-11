package ansible

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// 플레이북·컬렉션 목록의 구조 검사(Ansible 없이도 돈다): YAML 문법, 플레이 1개, 작업마다 모듈 하나와 이름,
// 쓰는 모듈은 community.vmware·ansible.builtin뿐, 자격 증명은 환경변수에서만 읽는다(평문 비밀번호 금지 — 오류 시험용 값 제외).
func TestPlaybookStructure(t *testing.T) {
	b, err := os.ReadFile("simulator.yml")
	if err != nil {
		t.Fatal(err)
	}
	var plays []map[string]any
	if err := yaml.Unmarshal(b, &plays); err != nil {
		t.Fatalf("simulator.yml YAML: %v", err)
	}
	if len(plays) != 1 {
		t.Fatalf("플레이 %d개, 기대 1", len(plays))
	}
	tasks, _ := plays[0]["tasks"].([]any)
	if len(tasks) < 20 {
		t.Fatalf("작업 %d개", len(tasks))
	}
	keys := map[string]bool{"name": true, "register": true, "ignore_errors": true}
	for i, x := range tasks {
		task, _ := x.(map[string]any)
		if _, ok := task["name"].(string); !ok {
			t.Errorf("작업 %d: name 없음", i)
		}
		var mods []string
		for k := range task {
			if !keys[k] {
				mods = append(mods, k)
			}
		}
		if len(mods) != 1 {
			t.Errorf("작업 %d(%v): 모듈 %v", i, task["name"], mods)
			continue
		}
		if !strings.HasPrefix(mods[0], "community.vmware.") && !strings.HasPrefix(mods[0], "ansible.builtin.") {
			t.Errorf("작업 %d: 허용하지 않는 모듈 %s", i, mods[0])
		}
	}
	for _, line := range strings.Split(string(b), "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "password:") || strings.HasPrefix(l, "url_password:") {
			if !strings.Contains(l, "lookup('env'") && !strings.Contains(l, "{{ nsx_password }}") && !strings.Contains(l, "wrong-password") {
				t.Errorf("자격 증명은 환경변수로만: %s", l)
			}
		}
	}
	var req map[string]any
	rb, _ := os.ReadFile("requirements.yml")
	if err := yaml.Unmarshal(rb, &req); err != nil || req["collections"] == nil {
		t.Fatalf("requirements.yml: %v", err)
	}
}
