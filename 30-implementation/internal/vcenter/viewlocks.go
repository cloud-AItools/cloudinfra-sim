package vcenter

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/types"
)

// vcsim(govmomi v0.56.0) 누수 보완(R-45 ④): 세션 범위 객체(ContainerView·ListView·PropertyFilter 등)는 세션
// 레지스트리에 있지만 메서드 호출의 객체 락은 전역 레지스트리(Map)의 locks 표에 만들어진다. DestroyView·
// DestroyPropertyFilter는 세션 레지스트리에서만 지우므로 전역 locks 항목이 영영 남는다(vcsim Context.WithLock의 TODO).
// 뷰를 만들고 지우는 클라이언트(govc, CMP 폴링, 관계 조정)가 많을수록 메모리가 늘어난다.
// 어댑터가 파괴 요청을 기록하고, 객체가 실제로 사라진 뒤 전역 Remove로 락 항목만 지운다. 전역에 있는 객체는 건드리지 않는다.
// 세션 종료(2026-10-11 추가): 객체를 파괴하지 않고 Logout·TerminateSession·유휴 만료로 끝난 세션의 객체도 같다 →
// 세션마다 호출을 받은(=락 항목이 생긴) 세션 객체를 기록해 두고, 세션이 끝나면 그 락 항목을 지운다.

// destroyMethods는 세션 범위 객체를 없애는 메서드다.
var destroyMethods = map[string]bool{"DestroyView": true, "DestroyPropertyFilter": true, "DestroyPropertyCollector": true}

// releaseDelays는 파괴 요청 뒤 락 항목을 지우려고 다시 확인하는 간격이다. 끝까지 객체가 남으면(파괴 실패) 그만둔다.
var releaseDelays = []time.Duration{50 * time.Millisecond, 500 * time.Millisecond, 5 * time.Second}

// sessionLocks는 세션 키 → 호출을 받은 세션 객체(→ 그 락 항목이 있는 전역 레지스트리)와 세션 레지스트리다.
// vcsim TerminateSession은 Logout과 달리 세션의 뷰 등 RegisterObject를 전역 변경 처리기에서 빼지 않아, 끝난 세션의 뷰가
// 이후 모든 변경 통지를 받으며 락 항목을 다시 만든다 → 종료 시 Logout과 같은 정리(전역 Remove)를 한다.
var sessionLocks = struct {
	sync.Mutex
	m    map[string]map[types.ManagedObjectReference]*simulator.Registry
	regs map[string]sessionReg
}{m: map[string]map[types.ManagedObjectReference]*simulator.Registry{}, regs: map[string]sessionReg{}}

type sessionReg struct{ session, global *simulator.Registry }

// sessionEndDelay는 세션 종료 요청 뒤 락 항목을 지우기까지 기다리는 시간이다(종료 처리가 끝난 뒤).
var sessionEndDelay = 500 * time.Millisecond

// noteSessionCall은 어댑터 훅(호출 전)에서 부른다. 세션 객체 호출을 기록하고, 파괴·세션 종료 뒤 전역 락 항목을 지운다.
func noteSessionCall(ctx *simulator.Context, method *simulator.Method) {
	if ctx.Session == nil || ctx.Map == nil {
		return
	}
	switch method.Name {
	case "Logout":
		releaseSession(ctx.Session.Key)
		return
	case "TerminateSession":
		if req, ok := method.Body.(*types.TerminateSession); ok {
			for _, id := range req.SessionId {
				releaseSession(id)
			}
		}
		return
	}
	key := ctx.Session.Key
	if key != "" && ctx.Session.Registry != nil {
		sessionLocks.Lock()
		if _, ok := sessionLocks.regs[key]; !ok {
			sessionLocks.regs[key] = sessionReg{session: ctx.Session.Registry, global: ctx.Map}
		}
		sessionLocks.Unlock()
	}
	ref := method.This
	if ctx.Map.Get(ref) != nil || ctx.Session.Registry.Get(ref) == nil {
		return // 전역 객체이거나 이미 없는 객체
	}
	sessionLocks.Lock()
	if destroyMethods[method.Name] {
		delete(sessionLocks.m[key], ref)
	} else if key != "" {
		if sessionLocks.m[key] == nil {
			sessionLocks.m[key] = map[types.ManagedObjectReference]*simulator.Registry{}
		}
		sessionLocks.m[key][ref] = ctx.Map
	}
	sessionLocks.Unlock()
	if destroyMethods[method.Name] {
		scheduleRelease(ctx.Map, ctx.Session.Registry, ref, 0)
	}
}

// releaseSession은 끝난 세션의 객체 락 항목을 지우고, 세션 객체(뷰 등 RegisterObject 포함)를 전역 변경 처리기에서 뺀다
// (Logout과 같은 정리 — TerminateSession·유휴 만료에는 vcsim이 하지 않는다). 전역 객체는 건드리지 않는다.
func releaseSession(key string) {
	sessionLocks.Lock()
	refs := sessionLocks.m[key]
	reg, hasReg := sessionLocks.regs[key]
	delete(sessionLocks.m, key)
	delete(sessionLocks.regs, key)
	sessionLocks.Unlock()
	if len(refs) == 0 && !hasReg {
		return
	}
	time.AfterFunc(sessionEndDelay, func() {
		remove := func(global *simulator.Registry, ref types.ManagedObjectReference) {
			if global.Get(ref) == nil {
				global.Remove(&simulator.Context{Context: context.Background(), Map: global}, ref)
			}
		}
		for ref, global := range refs {
			remove(global, ref)
		}
		if hasReg { // 세션이 끝났으므로 세션의 모든 객체(뷰·필터·수집기 등)를 정리한다. 전역 객체(싱글턴의 세션 사본)는 제외
			for _, obj := range reg.session.AllReference("") {
				remove(reg.global, obj.Reference())
			}
		}
	})
}

func scheduleRelease(global, session *simulator.Registry, ref types.ManagedObjectReference, attempt int) {
	if attempt >= len(releaseDelays) {
		return
	}
	// 별도 고루틴에서 지운다: 훅은 vcsim 호출 경로 안이라 다른 객체 락을 잡은 채일 수 있다.
	time.AfterFunc(releaseDelays[attempt], func() {
		if session.Get(ref) != nil {
			scheduleRelease(global, session, ref, attempt+1) // 아직 파괴 전
			return
		}
		if global.Get(ref) == nil {
			global.Remove(&simulator.Context{Context: context.Background(), Map: global}, ref)
		}
	})
}

// RegistryStats는 vcsim 전역 레지스트리의 객체 수와 락 항목 수다(장시간 운전 진단, R-45 ④).
func (v *VCenter) RegistryStats() (objects, locks int) {
	m := v.Model()
	if m == nil {
		return 0, 0
	}
	b, err := m.Map().MarshalJSON()
	if err != nil {
		return 0, 0
	}
	var s struct{ Objects, Locks int }
	_ = json.Unmarshal(b, &s)
	return s.Objects, s.Locks
}
