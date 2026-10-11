package vcenter

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Event·PropertyCollector 개선(WP 4.6, FR-VC-0509, FR-MET-0202, FR-BE-0611).
//
// 1) 데이터 버전(UpdateSet.version): vcsim은 버전을 발급하지 않는다(첫 응답 "-", 이후 요청 값을 그대로 반사, R-39).
//    API 문서(WaitForUpdatesEx·CheckForUpdates의 version 매개변수)는 버전이 "빈 문자열(초기) 또는 같은 세션·같은 Collector가
//    돌려준 데이터 버전"이어야 하며 아니면 InvalidCollectorVersion이라고 정한다. 세션·Collector별로 증가하는 버전을 발급하고,
//    발급하지 않은 버전은 거부한다. 이미 지난 버전을 다시 보내면 vcsim은 '마지막 호출 이후' 변경만 계산하므로 그 버전
//    이후의 변경을 다시 돌려주지는 않는다(제한, BEHAVIOR_SPEC §13).
// 2) 시간 필터: EventFilterSpec·TaskFilterSpec의 time.beginTime·endTime은 클라이언트가 보는 가상 시각이다. vcsim은 실제
//    시각과 비교하므로 요청 시각을 실제 시각 범위로 되돌린다(Phase 4a 알려진 제한 해소).

var (
	updateMethods   = map[string]bool{"WaitForUpdatesEx": true, "WaitForUpdates": true, "CheckForUpdates": true}
	timeFilterOps   = map[string]bool{"QueryEvents": true, "CreateCollectorForEvents": true, "CreateCollectorForTasks": true}
	collectorThisRe = regexp.MustCompile(`<_this[^>]*>([^<]+)</_this>`)
	reqVersionRe    = regexp.MustCompile(`<version>([^<]*)</version>`)
	setVersionRe    = regexp.MustCompile(`<returnval><version>[^<]*</version>`)
	beginRe         = regexp.MustCompile(`<beginTime>([^<]+)</beginTime>`)
	endRe           = regexp.MustCompile(`<endTime>([^<]+)</endTime>`)
)

const maxCollectorVersions = 1000 // 보존 상한(NFR-0503 계열, 가정)

type collectorVersions struct {
	mu    sync.Mutex
	state map[string]*collectorState // 세션 쿠키 + Collector → 상태
	order []string
}

type collectorState struct {
	seq    int
	issued map[string]bool
}

func newCollectorVersions() *collectorVersions {
	return &collectorVersions{state: map[string]*collectorState{}}
}

func collectorKey(r *http.Request, body []byte) string {
	sess := ""
	if c, err := r.Cookie("vmware_soap_session"); err == nil {
		sess = c.Value
	}
	this := ""
	if m := collectorThisRe.FindSubmatch(body); m != nil {
		this = string(m[1])
	}
	return sess + "/" + this
}

// check는 요청 버전이 유효한가(빈 문자열 또는 발급한 버전).
func (cv *collectorVersions) check(key string, body []byte) (string, bool) {
	m := reqVersionRe.FindSubmatch(body)
	if m == nil || len(m[1]) == 0 {
		return "", true
	}
	ver := string(m[1])
	cv.mu.Lock()
	defer cv.mu.Unlock()
	st := cv.state[key]
	return ver, st != nil && st.issued[ver]
}

// issue는 응답 UpdateSet의 버전을 새 버전으로 바꾼다.
func (cv *collectorVersions) issue(key string, resp []byte) []byte {
	if !setVersionRe.Match(resp) {
		return resp
	}
	cv.mu.Lock()
	st := cv.state[key]
	if st == nil {
		st = &collectorState{issued: map[string]bool{}}
		cv.state[key] = st
		cv.order = append(cv.order, key)
		if len(cv.order) > maxCollectorVersions {
			delete(cv.state, cv.order[0])
			cv.order = cv.order[1:]
		}
	}
	st.seq++
	ver := strconv.Itoa(st.seq)
	st.issued[ver] = true
	cv.mu.Unlock()
	done := false
	return setVersionRe.ReplaceAllFunc(resp, func(b []byte) []byte {
		if done {
			return b
		}
		done = true
		return []byte("<returnval><version>" + ver + "</version>")
	})
}

// shiftTimeFilter는 요청의 가상 시각 범위를 실제 시각 범위로 바꾼다.
func (v *VCenter) shiftTimeFilter(body []byte) []byte {
	conv := func(re *regexp.Regexp, tag string, f func(time.Time) time.Time) {
		body = re.ReplaceAllFunc(body, func(b []byte) []byte {
			m := re.FindSubmatch(b)
			t, err := time.Parse(time.RFC3339Nano, string(m[1]))
			if err != nil {
				return b
			}
			return []byte(fmt.Sprintf("<%s>%s</%s>", tag, f(t).UTC().Format(time.RFC3339Nano), tag))
		})
	}
	conv(beginRe, "beginTime", v.Clock.ToRealEarliest)
	conv(endRe, "endTime", v.Clock.ToRealLatest)
	return body
}
