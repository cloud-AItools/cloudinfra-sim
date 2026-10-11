package nsx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloudinfra-sim/internal/fault"
	"cloudinfra-sim/internal/fixture"
	"cloudinfra-sim/internal/profile"
	"cloudinfra-sim/internal/store"
)

// AuthFunc는 제품별 자격 증명 확인 함수다.
type AuthFunc func(product, user, password string) bool

// ComputeManagerInfo는 등록된 가상 vCenter 정보다(FR-NSX-0601). Thumbprint는 Gateway 서버 인증서 지문과 같아야 한다(NFR-0315).
type ComputeManagerInfo struct {
	Server     string
	Thumbprint string
}

// Engine은 NSX Manager·Policy API 엔진이다.
//
// 계약 근거: NSX-T 3.2.1 공식 스펙(SRC-NSX-321). 가정(미검증): ApiError의 error_code 값, PATCH의 오래된 _revision → 412,
// 삭제된 intent의 Realization 조회 → 404, /api/session/create(스펙 밖) 동작. BEHAVIOR_SPEC.md §4·§6 참조.
type Engine struct {
	prof    *profile.Set
	fix     *fixture.Fixture
	auth    AuthFunc
	cm      ComputeManagerInfo
	specs   []*Spec // [Manager, Policy]
	specDir string

	db *store.Store // 원본 저장소(SQLite). store 맵은 읽기 캐시이며 같은 락 안에서 함께 갱신한다(FR-BE-0303).

	mu           sync.Mutex
	store        map[string]map[string]any // 키: basePath + 상대 경로
	partialPatch bool                      // 시스템 설정 nsx-partial-patch-config(스펙 기본 false)
	realized     map[string]time.Time      // intent path → SUCCESS 예정 시각
	sessions     map[string]session        // JSESSIONID → 세션
	delay        time.Duration
	Now          func() time.Time // 시계(Virtual Clock, WP 4.10). nil이면 실제 시간

	onChange  []func(cause string)      // 관계 엔진 신호(OnChange). cause: "cid=<상관 ID>"
	sync      map[string]syncInfo       // Segment vCenter 동기화 상태(relations.go)
	derived   map[string]bool           // vCenter 파생 객체 키(시스템 소유, 저장소 미기록)
	alarms    map[string]map[string]any // intent path → NSX 알람(Realization 실패)
	realFault map[string]realFault      // Fault로 주입한 Realization 실패·정지(intent path)
	ruleSeq   int                       // 다음 Rule rule_id - 1(policy.go)
	Faults    *fault.Store              // NSX 의미 계층 Fault(WP 4.3)
	Log       *slog.Logger
	Role      func(product, user string) (role, scope string) // 역할(RBAC, WP 4.2). nil이면 검사 안 함
}

type session struct {
	last time.Time // 마지막 사용(가상 시각, 유휴 만료 FR-API-0209)
	user string
	xsrf string
}

const (
	managerBase = "/api/v1"
	policyBase  = "/policy/api/v1"
)

// New는 활성 프로파일의 스펙만 로드한다(FR-NSX-0105). specDir가 비면 프로파일 디렉터리에서 찾는다.
func New(prof *profile.Set, fix *fixture.Fixture, auth AuthFunc, cm ComputeManagerInfo, specDir string, db *store.Store) (*Engine, error) {
	if specDir == "" {
		specDir = prof.NSXDir
	}
	e := &Engine{prof: prof, fix: fix, auth: auth, cm: cm, db: db, specDir: specDir, delay: time.Duration(prof.NSX.Behavior.RealizationDelayMS) * time.Millisecond}
	for _, key := range []string{"manager", "policy"} {
		path := filepath.Join(specDir, prof.NSX.SpecFiles[key])
		s, err := Load(path)
		if err != nil {
			return nil, fmt.Errorf("NSX 스펙(%s)을 읽지 못했다: %w — 50-deployment/install/Get-NsxSpec.ps1로 내려받아 %s에 둔다(D11)", key, err, specDir)
		}
		e.specs = append(e.specs, s)
	}
	if err := e.Reset(); err != nil {
		return nil, err
	}
	return e, nil
}

// SetRealizationDelay는 Realization 지연을 바꾼다(FR-BE-0606). Admin API와 시험에서 사용한다.
func (e *Engine) SetRealizationDelay(d time.Duration) {
	e.mu.Lock()
	e.delay = d
	e.mu.Unlock()
}

// Reset은 NSX 상태를 Fixture로 되돌린다(TR-SCN-0102). 세션·시스템 설정도 초기화한다.
func (e *Engine) Reset() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.store = map[string]map[string]any{}
	e.realized = map[string]time.Time{}
	e.sync = map[string]syncInfo{}
	e.derived = map[string]bool{}
	e.alarms = map[string]map[string]any{}
	e.realFault = map[string]realFault{}
	e.ruleSeq = 1000
	e.sessions = map[string]session{}
	e.partialPatch = false
	now := float64(e.now().UnixMilli())
	put := func(base, rel string, obj map[string]any) {
		obj["_create_time"], obj["_last_modified_time"], obj["_revision"] = now, now, float64(0)
		obj["_create_user"], obj["_last_modified_user"] = "system", "system"
		if base == policyBase {
			obj["path"] = rel
			obj["relative_path"] = obj["id"]
			obj["parent_path"] = rel[:strings.LastIndex(rel, "/")]
			e.realized[rel] = time.Time{}
		}
		e.store[base+rel] = obj
	}
	seed := e.fix.Seed
	put(managerBase, "/fabric/compute-managers/"+uuidFrom(e.fix.Namespace, seed, "cm"), map[string]any{
		"id": uuidFrom(e.fix.Namespace, seed, "cm"), "display_name": e.cm.Server, "server": e.cm.Server, "origin_type": "vCenter",
		"resource_type": "ComputeManager",
		"credential":    map[string]any{"credential_type": "UsernamePasswordLoginCredential", "thumbprint": e.cm.Thumbprint},
	})
	for i, tz := range e.fix.NSX.TransportZones {
		id := uuidFrom(e.fix.Namespace, seed, "tz"+strconv.Itoa(i))
		put(managerBase, "/transport-zones/"+id, map[string]any{
			"id": id, "display_name": tz.DisplayName, "transport_type": tz.TransportType, "resource_type": "TransportZone",
		})
		put(policyBase, "/infra/sites/default/enforcement-points/default/transport-zones/"+tz.ID, map[string]any{
			"id": tz.ID, "display_name": tz.DisplayName, "tz_type": tz.TransportType + "_BACKED", "resource_type": "PolicyTransportZone",
			"realization_id": id,
		})
	}
	for _, t1 := range e.fix.NSX.Tier1s {
		put(policyBase, "/infra/tier-1s/"+t1, map[string]any{"id": t1, "display_name": t1, "resource_type": "Tier1"})
	}
	for i := 1; i <= e.fix.NSX.Segments; i++ {
		id := fmt.Sprintf("seg-%02d", i)
		put(policyBase, "/infra/segments/"+id, map[string]any{
			"id": id, "display_name": id, "resource_type": "Segment", "admin_state": "UP",
			"transport_zone_path": "/infra/sites/default/enforcement-points/default/transport-zones/tz-overlay",
			"unique_id":           uuidFrom(e.fix.Namespace, seed, "seg"+id),
		})
	}
	e.seedPolicyLocked(put)
	// 시드 상태를 저장소에 한 트랜잭션으로 기록한다.
	ctx := context.Background()
	if err := e.db.Reset(ctx); err != nil {
		return err
	}
	tx, err := e.db.BeginNSX(ctx)
	if err != nil {
		return err
	}
	for k, obj := range e.store {
		if err := tx.Put(k, obj); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// uuidFrom은 환경 Namespace·Seed 기반 결정론적 UUID 형식 문자열이다(FR-DM-0207 환경 간 분리, FR-DM-0210 재현).
func uuidFrom(ns string, seed int64, key string) string {
	h := fnv64(fmt.Sprintf("%s/%d/%s", ns, seed, key))
	h2 := fnv64(fmt.Sprintf("%s/%d/%s", key, seed, ns))
	b := fmt.Sprintf("%016x%016x", h, h2)
	return b[0:8] + "-" + b[8:12] + "-4" + b[13:16] + "-a" + b[17:20] + "-" + b[20:32]
}

func fnv64(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

// apiError는 스펙 ApiError 형식으로 오류를 쓴다(FR-NSX-0416). error_code 값은 가정이다.
func apiError(w http.ResponseWriter, status int, module, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error_code":    status, // 가정: 상황별 실제 error_code 값은 근거 확보 후 errors.yaml로 대체
		"error_message": msg,
		"module_name":   module,
	})
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sortedKeys는 결정론적 목록 순서를 위해 사용한다.
func sortedKeys(m map[string]map[string]any, prefix string, depth int) []string {
	var out []string
	for k := range m {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		if strings.Count(strings.TrimPrefix(k, prefix), "/") != depth {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// now는 현재 가상 시각이다(Realization 지연·메타데이터 시각, FR-BE-0602 ~ 0604).
func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}
