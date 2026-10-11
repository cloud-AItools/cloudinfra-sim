// Package fault는 공통 Fault Rule 모델과 규칙 저장소다(PRD-1 FR-FLT-0201 ~ 0206, WP 3.8·4.3).
//
// 규칙 = 조건(Match) + 동작(Action) + 횟수(Count) + Seed. 계층(FR-FLT-0204)마다 적용 위치가 다르다:
//   - transport(Gateway): delay·status·unavailable·timeout·reset
//   - vcenter(Behavior Adapter, 요청 경로 안): task_fail·soap_fault·task_delay, 인프라 상태(host_disconnect·
//     host_maintenance·datastore_full: 규칙이 있는 동안 상태 유지, 제거하면 되돌림)
//   - nsx(NSX 엔진): realization_fail·realization_hang·revision_conflict
//   - session(Gateway): expire(일치한 요청의 세션을 만료시킨 뒤 전달 → 인증 실패)
//
// 조건(FR-FLT-0202): 제품·HTTP 메서드·경로 접두어·SOAP 메서드·리소스 ID·사용자·N번째 호출(nth)·호출 순서(after:
// 이 호출이 한 번 관측된 뒤부터)·가상 시각 범위(not_before·not_after)·확률(Seed 고정 난수). 관리는 Admin API로만 한다
// (FR-FLT-0206). 규칙이 없으면 정상 요청에 오류를 만들지 않는다(FR-FLT-0207).
package fault

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	LayerTransport = "transport"
	LayerVCenter   = "vcenter"
	LayerNSX       = "nsx"
	LayerSession   = "session"

	// transport
	ActionDelay       = "delay"
	ActionStatus      = "status"
	ActionUnavailable = "unavailable"
	ActionTimeout     = "timeout" // 응답 없이 delay_ms 동안 붙잡은 뒤 연결을 닫는다(FR-FLT-0118)
	ActionReset       = "reset"   // 응답 없이 연결을 끊는다(FR-FLT-0119)
	// vcenter
	ActionTaskFail        = "task_fail"
	ActionSOAPFault       = "soap_fault"
	ActionTaskDelay       = "task_delay"
	ActionHostDisconnect  = "host_disconnect"
	ActionHostMaintenance = "host_maintenance"
	ActionDatastoreFull   = "datastore_full"
	// nsx
	ActionRealizationFail = "realization_fail"
	ActionRealizationHang = "realization_hang"
	ActionRevisionConfict = "revision_conflict"
	// session
	ActionExpire = "expire"

	MaxDelayMS = 600000
)

var layerActions = map[string][]string{
	LayerTransport: {ActionDelay, ActionStatus, ActionUnavailable, ActionTimeout, ActionReset},
	LayerVCenter:   {ActionTaskFail, ActionSOAPFault, ActionTaskDelay, ActionHostDisconnect, ActionHostMaintenance, ActionDatastoreFull},
	LayerNSX:       {ActionRealizationFail, ActionRealizationHang, ActionRevisionConfict},
	LayerSession:   {ActionExpire},
}

// StateActions는 규칙이 있는 동안 인프라 상태를 바꾸는 동작이다(요청마다 적용하지 않음).
var StateActions = map[string]bool{ActionHostDisconnect: true, ActionHostMaintenance: true, ActionDatastoreFull: true}

// Match는 규칙 조건이다. 빈 필드는 모두 일치로 본다.
type Match struct {
	Product    string     `json:"product,omitempty"`     // vcenter | nsx | esx
	Method     string     `json:"method,omitempty"`      // HTTP 메서드
	PathPrefix string     `json:"path_prefix,omitempty"` // 요청 경로(NSX는 Policy·Manager 경로) 접두어
	SOAPMethod string     `json:"soap_method,omitempty"` // 예: PowerOnVM_Task
	Resource   string     `json:"resource,omitempty"`    // MoRef 값(예: vm-57) 또는 NSX intent 경로
	User       string     `json:"user,omitempty"`        // 요청 사용자
	Nth        int        `json:"nth,omitempty"`         // N번째 일치 호출부터 적용(1부터)
	After      string     `json:"after,omitempty"`       // 이 SOAP 메서드 또는 "METHOD 경로접두어"가 관측된 뒤부터
	NotBefore  *time.Time `json:"not_before,omitempty"`  // 가상 시각 범위
	NotAfter   *time.Time `json:"not_after,omitempty"`
}

// Action은 규칙 동작이다.
type Action struct {
	Type          string `json:"type"`
	DelayMS       int    `json:"delay_ms,omitempty"`
	Status        int    `json:"status,omitempty"`
	RetryAfterSec int    `json:"retry_after_sec,omitempty"`
	Fault         string `json:"fault,omitempty"`   // vcenter: Fault 타입 이름(예: InvalidPowerState)
	Message       string `json:"message,omitempty"` // 오류 메시지(NSX Realization 오류 등)
}

// Rule은 공통 Fault Rule이다.
type Rule struct {
	ID          string    `json:"id"`
	Layer       string    `json:"layer"`
	Match       Match     `json:"match"`
	Action      Action    `json:"action"`
	Count       int       `json:"count"`                 // 적용 횟수. 0이면 제거할 때까지
	Probability float64   `json:"probability,omitempty"` // 0이면 1(항상)
	Seed        int64     `json:"seed"`
	Hits        int       `json:"hits"`  // 적용한 횟수(읽기 전용)
	Seen        int       `json:"seen"`  // 조건에 일치한 호출 수(읽기 전용, nth 판정)
	Armed       bool      `json:"armed"` // after 조건 충족 여부(읽기 전용)
	Created     time.Time `json:"created"`

	rnd *mrand.Rand
}

// Request는 규칙 판정에 쓰는 요청 정보다.
type Request struct {
	Product, Method, Path, SOAPMethod, Resource, User string
	Now                                               time.Time // 가상 시각
}

// FaultTypeOK는 vcenter Fault 이름 검증 함수다(vcenter 패키지가 설정, 순환 의존 회피).
var FaultTypeOK = func(string) bool { return true }

// Store는 활성 규칙 목록이다.
type Store struct {
	mu       sync.Mutex
	rules    map[string]*Rule
	seq      int
	onChange []func(added, removed []Rule)
}

func NewStore() *Store { return &Store{rules: map[string]*Rule{}} }

// OnChange는 규칙 추가·제거 때 부를 함수를 등록한다(인프라 상태 동작 적용·해제).
func (s *Store) OnChange(fn func(added, removed []Rule)) {
	s.mu.Lock()
	s.onChange = append(s.onChange, fn)
	s.mu.Unlock()
}

func (s *Store) notify(added, removed []Rule) {
	s.mu.Lock()
	fns := append([]func(added, removed []Rule){}, s.onChange...)
	s.mu.Unlock()
	for _, f := range fns {
		f(added, removed)
	}
}

// Validate는 규칙 입력을 검사한다.
func (r *Rule) Validate() error {
	var errs []string
	if r.Layer == "" {
		r.Layer = LayerTransport
	}
	acts, ok := layerActions[r.Layer]
	if !ok {
		errs = append(errs, "layer: transport | vcenter | nsx | session")
	} else if !contains(acts, r.Action.Type) {
		errs = append(errs, fmt.Sprintf("action.type(%s 계층): %s", r.Layer, strings.Join(acts, " | ")))
	}
	switch r.Match.Product {
	case "", "vcenter", "nsx", "esx":
	default:
		errs = append(errs, "match.product: vcenter | nsx | esx")
	}
	a := &r.Action
	switch a.Type {
	case ActionDelay, ActionTimeout:
		if a.DelayMS <= 0 || a.DelayMS > MaxDelayMS {
			errs = append(errs, fmt.Sprintf("action.delay_ms: 1~%d", MaxDelayMS))
		}
	case ActionTaskDelay:
		if a.DelayMS <= 0 || a.DelayMS > MaxDelayMS {
			errs = append(errs, fmt.Sprintf("action.delay_ms: 1~%d", MaxDelayMS))
		}
	case ActionStatus:
		if a.Status < 400 || a.Status > 599 {
			errs = append(errs, "action.status: 400~599")
		}
	case ActionUnavailable:
		a.Status = 503
		if a.RetryAfterSec < 0 {
			errs = append(errs, "action.retry_after_sec: 0 이상")
		}
	case ActionTaskFail, ActionSOAPFault:
		if a.Fault == "" || !FaultTypeOK(a.Fault) {
			errs = append(errs, "action.fault: vim25 MethodFault 타입 이름(예: InvalidPowerState)")
		}
		if a.DelayMS < 0 || a.DelayMS > MaxDelayMS {
			errs = append(errs, "action.delay_ms: 0~600000(Task 실행 중 실패 시점)")
		}
	case ActionHostDisconnect, ActionHostMaintenance, ActionDatastoreFull:
		if r.Match.Resource == "" {
			errs = append(errs, "match.resource: 대상 MoRef 값(host-… 또는 datastore-…)")
		}
	}
	if r.Count < 0 || r.Match.Nth < 0 {
		errs = append(errs, "count·nth: 0 이상")
	}
	if r.Probability < 0 || r.Probability > 1 {
		errs = append(errs, "probability: 0~1")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Add는 규칙을 검사해 추가하고 ID를 붙인다.
func (s *Store) Add(r Rule) (Rule, error) {
	if err := r.Validate(); err != nil {
		return Rule{}, err
	}
	s.mu.Lock()
	s.seq++
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	r.ID = fmt.Sprintf("fault-%d-%s", s.seq, hex.EncodeToString(b))
	r.Hits, r.Seen, r.Armed = 0, 0, r.Match.After == ""
	r.Created = time.Now().UTC()
	r.rnd = mrand.New(mrand.NewSource(r.Seed))
	s.rules[r.ID] = &r
	out := r
	s.mu.Unlock()
	s.notify([]Rule{out}, nil)
	return out, nil
}

// Remove는 규칙 하나를 지운다. 없으면 false.
func (s *Store) Remove(id string) bool {
	s.mu.Lock()
	r, ok := s.rules[id]
	delete(s.rules, id)
	s.mu.Unlock()
	if ok {
		s.notify(nil, []Rule{*r})
	}
	return ok
}

// Clear는 모든 규칙을 지운다(환경 초기화 포함).
func (s *Store) Clear() {
	s.mu.Lock()
	var removed []Rule
	for _, r := range s.rules {
		removed = append(removed, *r)
	}
	s.rules = map[string]*Rule{}
	s.mu.Unlock()
	if len(removed) > 0 {
		s.notify(nil, removed)
	}
}

// List는 규칙을 생성 순으로 돌려준다.
func (s *Store) List() []Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Rule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, *r)
	}
	sortRules(out)
	return out
}

func sortRules(out []Rule) {
	sort.Slice(out, func(i, j int) bool {
		return out[i].Created.Before(out[j].Created) || out[i].Created.Equal(out[j].Created) && out[i].ID < out[j].ID
	})
}

// Observe는 모든 요청을 관측해 after 조건을 충족시킨다(호출 순서 조건).
func (s *Store) Observe(q Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rules {
		if r.Armed || r.Match.After == "" {
			continue
		}
		a := r.Match.After
		if a == q.SOAPMethod || strings.Contains(a, " ") && strings.HasPrefix(q.Method+" "+q.Path, a) {
			r.Armed = true
		}
	}
}

// Apply는 계층 layer에서 요청에 맞는 규칙 하나를 골라 적용 횟수를 올린다(생성 순 첫 규칙). 없으면 ok=false.
// 인프라 상태 동작은 요청마다 적용하지 않으므로 고르지 않는다. Count를 다 쓴 규칙은 남겨 두되 적용하지 않는다.
func (s *Store) Apply(layer string, q Request) (Rule, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var cand []*Rule
	for _, r := range s.rules {
		if r.Layer == layer && !StateActions[r.Action.Type] && r.matches(q) {
			cand = append(cand, r)
		}
	}
	sort.Slice(cand, func(i, j int) bool { return cand[i].Created.Before(cand[j].Created) })
	for _, r := range cand {
		r.Seen++
		if !r.Armed || r.Match.Nth > 0 && r.Seen < r.Match.Nth || r.Count > 0 && r.Hits >= r.Count {
			continue
		}
		if r.Probability > 0 && r.Probability < 1 && r.rnd.Float64() >= r.Probability {
			continue
		}
		r.Hits++
		return *r, true
	}
	return Rule{}, false
}

// Active는 계층·동작이 같은 활성 규칙 목록이다(인프라 상태 동작 조회용).
func (s *Store) Active(layer, action string) []Rule {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Rule
	for _, r := range s.rules {
		if r.Layer == layer && r.Action.Type == action {
			out = append(out, *r)
		}
	}
	sortRules(out)
	return out
}

func (r *Rule) matches(q Request) bool {
	m := r.Match
	if m.NotBefore != nil && !q.Now.IsZero() && q.Now.Before(*m.NotBefore) || m.NotAfter != nil && !q.Now.IsZero() && q.Now.After(*m.NotAfter) {
		return false
	}
	return (m.Product == "" || m.Product == q.Product) &&
		(m.Method == "" || strings.EqualFold(m.Method, q.Method)) &&
		(m.PathPrefix == "" || strings.HasPrefix(q.Path, m.PathPrefix)) &&
		(m.SOAPMethod == "" || m.SOAPMethod == q.SOAPMethod) &&
		(m.Resource == "" || m.Resource == q.Resource) &&
		(m.User == "" || m.User == q.User)
}
