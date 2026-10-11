package performance

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

// govcsims는 데이터 디렉터리의 tmp/<pid> 아래 vcsim 임시 디렉터리 목록이다.
func govcsims(t *testing.T, data string, pid int) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(data, "tmp", strconv.Itoa(pid), "govcsim-*"))
	return m
}

// T-NFR-0508: 일반 실행이 강제 종료(작업 관리자·kill -9)돼도 vcsim 임시 파일(govcsim-*)이 쌓이지 않는다 —
// 임시 파일은 <data-dir>/tmp/<pid>에 만들고, 다음 기동이 실행 중이 아닌 pid의 것을 지운다. 정상 종료는 바로 지운다.
func TestForcedKillTempCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("실행파일을 빌드한다")
	}
	data := t.TempDir()
	fx := filepath.Join(testenv.Repo(), "30-implementation", "data", "fixtures", "default-small.yaml")
	p1 := startIn(t, data, fx, "--run-profile", "lite")
	pid1 := p1.cmd.Process.Pid
	if len(govcsims(t, data, pid1)) == 0 {
		t.Fatalf("vcsim 임시 디렉터리가 %s 아래에 없다", filepath.Join(data, "tmp", strconv.Itoa(pid1)))
	}
	// 강제 종료: 정리 기회 없음 → 남는다
	_ = p1.cmd.Process.Kill()
	_, _ = p1.cmd.Process.Wait()
	if len(govcsims(t, data, pid1)) == 0 {
		t.Fatal("강제 종료 뒤 남아 있어야 할 임시 디렉터리가 없다(시험 전제 확인)")
	}
	// 같은 데이터 디렉터리로 다시 기동 → 이전 실행의 것을 지운다
	p2 := startIn(t, data, fx, "--run-profile", "lite")
	if _, err := os.Stat(filepath.Join(data, "tmp", strconv.Itoa(pid1))); !os.IsNotExist(err) {
		t.Fatalf("강제 종료된 실행(pid %d)의 임시 디렉터리가 재기동 뒤에도 남았다", pid1)
	}
	pid2 := p2.cmd.Process.Pid
	if len(govcsims(t, data, pid2)) == 0 {
		t.Fatal("재기동한 실행의 vcsim 임시 디렉터리가 tmp/<pid> 아래에 없다")
	}
	// 정상 종료(신호)는 바로 지운다. Windows는 다른 프로세스에 Ctrl+C를 보낼 수 없어 단위 시험(tempdir.TestIsolate)으로 대신한다.
	if runtime.GOOS != "windows" {
		_ = p2.cmd.Process.Signal(os.Interrupt)
		_, _ = p2.cmd.Process.Wait()
		if _, err := os.Stat(filepath.Join(data, "tmp", strconv.Itoa(pid2))); !os.IsNotExist(err) {
			t.Fatal("정상 종료 뒤에도 임시 디렉터리가 남았다")
		}
	}
}
