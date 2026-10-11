package contract

import (
	"testing"

	"cloudinfra-sim/internal/config"
	"cloudinfra-sim/test/internal/testenv"
)

// T-TR-SCN-0105 T-FR-DM-0306: 적재한 모델(Fixture 캐시·이전 복원)로 만든 Snapshot도 복원된다. vcsim Load가 vcsim.home 설정을
// 하나 더 덧붙여 저장 때 홈 경로 정리가 틀렸고, 복원이 사용 중인 홈 디렉터리를 다시 만들려다 실패했다(2026-10-11 수정).
func TestRestoreChainFromLoadedModel(t *testing.T) {
	e := testenv.Start(t, func(c *config.Config) { c.FixtureCache = "on" })
	if err := e.Sim.Reset(); err != nil { // 캐시로 적재한 모델
		t.Fatal(err)
	}
	if st := e.Sim.VC.LastCache(); st.Mode != "hit" {
		t.Fatalf("cache %+v", st)
	}
	for _, name := range []string{"c1", "c2", "c3"} { // 적재 모델 → Snapshot → 복원 → 그 모델로 다시 Snapshot …
		if _, err := e.Sim.CreateSnapshot(name); err != nil {
			t.Fatalf("snapshot %s: %v", name, err)
		}
		res, err := e.Sim.Restore(name)
		if err != nil {
			t.Fatalf("restore %s: %v", name, err)
		}
		if !res.Consistent {
			t.Fatalf("restore %s: 불일치 %v", name, res.Issues)
		}
	}
}
