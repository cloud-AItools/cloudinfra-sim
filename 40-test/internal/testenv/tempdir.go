package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"cloudinfra-sim/internal/tempdir"
)

// 시험 실행파일 전용 임시 디렉터리. vcsim은 모델마다 데이터스토어 파일을 OS 임시 디렉터리(govcsim-*)에 만들고 정상
// 종료 때만 지운다. 시험이 Timeout으로 강제 종료되거나 벤치마크가 시뮬레이터 프로세스를 죽이면 그대로 남아 쌓였다
// (2026-10-11 정리: 136개, 407 MB). 시험 실행파일마다 TMP·TEMP·TMPDIR를 <OS 임시>/cloudinfra-sim-test/<pid>로 바꿔
// 그 안에 모으고(자식 프로세스도 환경을 물려받는다), 끝날 때 지운다. 강제 종료로 남은 것은 다음 실행이 지운다
// (pid가 살아 있지 않은 디렉터리만). 시험 밖에서 실행 중인 시뮬레이터의 govcsim-*는 건드리지 않는다.

// 격리·정리 자체는 제품 실행과 같은 internal/tempdir를 쓴다(cmd/cloudinfra-sim은 <data-dir>/tmp).

// TempRootName은 시험 임시 디렉터리들의 상위 디렉터리 이름이다.
const TempRootName = "cloudinfra-sim-test"

// Main은 패키지의 TestMain에서 부른다: func TestMain(m *testing.M) { testenv.Main(m) }
func Main(m *testing.M) {
	os.Exit(runIsolated(m))
}

func runIsolated(m *testing.M) int {
	_, _, cleanup, err := tempdir.Isolate(filepath.Join(os.TempDir(), TempRootName))
	if err != nil {
		return m.Run() // 격리 실패: 기존 위치로 그대로 실행
	}
	defer cleanup()
	return m.Run()
}
