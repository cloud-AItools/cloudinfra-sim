package gateway

import (
	"fmt"
	"net/http"
	"testing"
)

// T-NFR-0501 / T-NFR-0504: 호출 기록은 링버퍼 한도를 넘지 않고 버린 건수를 표시한다.
func TestTraceRingBuffer(t *testing.T) {
	tr := NewTrace(3)
	for i := 0; i < 5; i++ {
		tr.Add(TraceEntry{Path: fmt.Sprint(i)})
	}
	got, dropped := tr.Recent(0)
	if len(got) != 3 || got[0].Path != "2" || got[2].Path != "4" || dropped != 2 {
		t.Fatalf("ring buffer: %+v dropped=%d", got, dropped)
	}
}

// T-NFR-0503: 미지원 API 집계 키 수에 상한을 두고 넘치면 기타 버킷으로 모은다.
func TestUnsupportedKeyCap(t *testing.T) {
	u := NewUnsupported(2)
	for _, k := range []string{"a", "b", "c", "a", "d"} {
		u.Add(k, "7.0.3.0")
	}
	stats, other := u.Snapshot()
	if len(stats) != 2 || other != 2 || stats[0].Key != "a" || stats[0].Count != 2 {
		t.Fatalf("stats=%+v other=%d", stats, other)
	}
}

// T-NFR-0307: 로그에는 인증 헤더·세션 값이 남지 않는다.
func TestMaskHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Basic YWRtaW46c2VjcmV0")
	h.Set("Cookie", "vmware_soap_session=abc")
	h.Set("vmware-api-session-id", "token")
	h.Set("X-XSRF-TOKEN", "xsrf")
	h.Set("SOAPAction", "urn:vim25/7.0.3.0")
	m := maskHeaders(h)
	for _, k := range []string{"authorization", "cookie", "vmware-api-session-id", "x-xsrf-token"} {
		if m[k] != "***" {
			t.Fatalf("%s not masked: %q", k, m[k])
		}
	}
	if m["soapaction"] != "urn:vim25/7.0.3.0" {
		t.Fatalf("soapaction should be kept: %v", m)
	}
}
