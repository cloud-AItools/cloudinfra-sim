package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T-TR-SCN-0302: 선언형 시나리오 파일 검사(알 수 없는 키, 단계 동작 하나, 이름 중복)와 배포 시나리오 적재.
func TestLoadValidation(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, err := Load(write("a.yaml", "name: a\nsteps:\n  - {name: x, stage: workflow, wait_ms: 10, unknown: 1}\n")); err == nil {
		t.Fatal("unknown key must be rejected")
	}
	if _, err := Load(write("b.yaml", "name: b\nsteps:\n  - {name: x, stage: workflow, wait_ms: 10, soap: {action: login}}\n")); err == nil || !strings.Contains(err.Error(), "하나만") {
		t.Fatalf("two actions: %v", err)
	}
	if _, err := Load(write("c.yaml", "name: c\nsteps: []\n")); err == nil {
		t.Fatal("empty steps")
	}
	d2 := t.TempDir()
	for _, n := range []string{"1.yaml", "2.yaml"} {
		_ = os.WriteFile(filepath.Join(d2, n), []byte("name: same\nsteps:\n  - {name: x, stage: workflow, wait_ms: 1}\n"), 0o600)
	}
	if _, err := LoadDir(d2); err == nil {
		t.Fatal("duplicate names")
	}
	files, err := LoadDir(filepath.Join("..", "..", "data", "scenarios"))
	if err != nil || len(files) < 7 {
		t.Fatalf("shipped scenarios: %d %v", len(files), err)
	}
}

// T-TR-SCN-0301: JSON 경로(배열 길이 #, 정수 정규화)
func TestJSONPath(t *testing.T) {
	doc := map[string]any{"a": []any{map[string]any{"b": float64(3)}, "x"}}
	if v, ok := jsonPath(doc, "a.#"); !ok || v != 2 {
		t.Fatalf("len: %v", v)
	}
	if v, ok := jsonPath(doc, "a.0.b"); !ok || v != int64(3) {
		t.Fatalf("int: %v", v)
	}
	if _, ok := jsonPath(doc, "a.5"); ok {
		t.Fatal("out of range")
	}
}
