package contract

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

// 시험 실행파일의 임시 파일 격리(TestMain → testenv.Main): vcsim 데이터스토어 디렉터리(govcsim-*)가 시험 전용
// 디렉터리(<OS 임시>/cloudinfra-sim-test/<pid>) 안에 만들어져, 시험이 강제 종료돼도 다음 실행이 정리할 수 있다.
func TestVcsimTempIsolated(t *testing.T) {
	want := filepath.Join(testenv.TempRootName, strconv.Itoa(os.Getpid()))
	if !strings.Contains(os.TempDir(), want) {
		t.Fatalf("os.TempDir() = %s, %s 아래여야 한다", os.TempDir(), want)
	}
	e := testenv.Start(t, nil)
	home := ""
	for _, o := range e.Sim.VC.Model().Map().OptionManager().Setting {
		if ov := o.GetOptionValue(); ov.Key == "vcsim.home" {
			home, _ = ov.Value.(string)
		}
	}
	if home == "" || !strings.Contains(filepath.Clean(home), want) {
		t.Fatalf("vcsim.home = %q, 시험 전용 임시 디렉터리(%s) 아래여야 한다", home, want)
	}
}
