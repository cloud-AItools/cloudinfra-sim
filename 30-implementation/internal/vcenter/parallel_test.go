package vcenter

import (
	"os"
	"path/filepath"
	"testing"
)

// 데이터스토어 묶음은 파일·빈 디렉터리를 그대로 되살리고 .lck는 빼며, 묶음 밖 경로는 거부한다.
func TestTarTreeRoundTrip(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	files := map[string]string{"vm1/vm1.vmx": "config", "vm1/disk1.vmdk": "desc", "vm1/disk1-flat.vmdk": "", "vm2/vm2.nvram": "nv"}
	for p, c := range files {
		_ = os.MkdirAll(filepath.Join(src, filepath.Dir(p)), 0o700)
		if err := os.WriteFile(filepath.Join(src, p), []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.MkdirAll(filepath.Join(src, "empty"), 0o700)
	_ = os.WriteFile(filepath.Join(src, "vm1", "vm1.vmx.lck"), []byte("x"), 0o600)
	tarPath := filepath.Join(t.TempDir(), "ds.tar")
	if err := tarTree(src, tarPath); err != nil {
		t.Fatal(err)
	}
	if err := untarTree(tarPath, dst); err != nil {
		t.Fatal(err)
	}
	for p, c := range files {
		b, err := os.ReadFile(filepath.Join(dst, p))
		if err != nil || string(b) != c {
			t.Errorf("%s: %q, %v", p, b, err)
		}
	}
	if st, err := os.Stat(filepath.Join(dst, "empty")); err != nil || !st.IsDir() {
		t.Error("빈 디렉터리가 없다")
	}
	if _, err := os.Stat(filepath.Join(dst, "vm1", "vm1.vmx.lck")); err == nil {
		t.Error(".lck가 묶였다")
	}
	// 같은 내용이면 같은 묶음(정렬된 순서)
	tar2 := filepath.Join(t.TempDir(), "ds2.tar")
	_ = tarTree(src, tar2)
	a, _ := os.ReadFile(tarPath)
	b, _ := os.ReadFile(tar2)
	if string(a) != string(b) {
		t.Error("같은 내용인데 묶음이 다르다")
	}
}
