package footprint

import (
	"testing"

	"cloudinfra-sim/test/internal/testenv"
)

// TestMain은 이 시험 실행파일과 자식 시뮬레이터의 임시 파일(vcsim govcsim-* 등)을 전용 디렉터리에 모으고 끝날 때 지운다.
func TestMain(m *testing.M) { testenv.Main(m) }
