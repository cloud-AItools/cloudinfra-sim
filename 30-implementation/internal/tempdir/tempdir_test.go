package tempdir

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Sweep는 실행 중이 아닌 pid의 디렉터리만 지운다(자기 자신·숫자가 아닌 이름은 남긴다).
func TestSweep(t *testing.T) {
	base := t.TempDir()
	dead := filepath.Join(base, "2147483646") // 존재하지 않는 pid
	self := filepath.Join(base, strconv.Itoa(os.Getpid()))
	other := filepath.Join(base, "keep-me")
	for _, d := range []string{dead, self, other} {
		if err := os.MkdirAll(filepath.Join(d, "govcsim-1", "DC0-LocalDS_0"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	removed := Sweep(base)
	if len(removed) != 1 || removed[0] != dead {
		t.Fatalf("removed = %v, want [%s]", removed, dead)
	}
	for _, d := range []string{self, other} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("%s를 지웠다", d)
		}
	}
	if Sweep(filepath.Join(base, "missing")) != nil {
		t.Error("없는 디렉터리에서 무언가를 지웠다")
	}
}

// Isolate는 임시 디렉터리 환경변수를 <base>/<pid>로 바꾸고, cleanup이 되돌리며 디렉터리를 지운다.
func TestIsolate(t *testing.T) {
	base := t.TempDir()
	before := os.TempDir()
	dir, _, cleanup, err := Isolate(base)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, strconv.Itoa(os.Getpid())); dir != want || os.TempDir() != want {
		t.Fatalf("dir %s, os.TempDir %s, want %s", dir, os.TempDir(), want)
	}
	f, err := os.CreateTemp("", "govcsim-")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if filepath.Dir(f.Name()) != dir {
		t.Fatalf("임시 파일 %s가 격리 디렉터리 밖에 만들어졌다", f.Name())
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup 뒤에도 %s가 남았다", dir)
	}
	if os.TempDir() != before {
		t.Fatalf("환경변수가 되돌려지지 않았다: %s", os.TempDir())
	}
}
