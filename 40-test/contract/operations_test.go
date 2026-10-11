// 운영 기능 계약 테스트: NSX 세션·XSRF(FR-API-0207), 인증서 교체(NFR-0303), 호출 기록(TR-SCN-0111), 로그 형식(NFR-0405).
package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"cloudinfra-sim/internal/certs"
	"cloudinfra-sim/test/internal/testenv"
)

// T-FR-API-0207: POST /api/session/create → JSESSIONID 쿠키 + X-XSRF-TOKEN. 변경 요청은 XSRF가 있어야 한다.
// 계약 근거 미확보(스펙 밖, NSX-001B) — 구현 동작 확인용이며 Integration Tested 승격 대상이 아니다.
func TestNSXSessionAndXSRF(t *testing.T) {
	e := testenv.Start(t, nil)
	u := e.User("nsx")
	form := url.Values{"j_username": {u.Name}, "j_password": {u.Password}}
	res, err := e.Client.Post("https://nsx."+testenv.Domain+"/api/session/create", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	xsrf := res.Header.Get("X-XSRF-TOKEN")
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "JSESSIONID" {
			cookie = c
		}
	}
	if res.StatusCode != 200 || xsrf == "" || cookie == nil {
		t.Fatalf("session create: %d xsrf=%q cookie=%v", res.StatusCode, xsrf, cookie)
	}
	do := func(method, path string, body string, withXSRF bool) int {
		req, _ := http.NewRequest(method, "https://nsx."+testenv.Domain+path, strings.NewReader(body))
		req.AddCookie(cookie)
		req.Header.Set("Content-Type", "application/json")
		if withXSRF {
			req.Header.Set("X-XSRF-TOKEN", xsrf)
		}
		res, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if code := do(http.MethodGet, "/api/v1/node/version", "", false); code != 200 {
		t.Fatalf("GET with session cookie: %d", code)
	}
	if code := do(http.MethodPatch, "/policy/api/v1/infra/segments/xsrf-test", `{"display_name":"x"}`, false); code != http.StatusForbidden {
		t.Fatalf("PATCH without XSRF must be 403: %d", code)
	}
	if code := do(http.MethodPatch, "/policy/api/v1/infra/segments/xsrf-test", `{"display_name":"x"}`, true); code != 200 {
		t.Fatalf("PATCH with XSRF: %d", code)
	}
	bad := url.Values{"j_username": {u.Name}, "j_password": {"wrong"}}
	res, _ = e.Client.Post("https://nsx."+testenv.Domain+"/api/session/create", "application/x-www-form-urlencoded", strings.NewReader(bad.Encode()))
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong password session create: %d", res.StatusCode)
	}
}

// T-NFR-0303: 인증서는 명시적 명령으로만 교체되고, 교체하면 새 지문이 나온다(CA는 유지).
func TestCertificateRotation(t *testing.T) {
	dir := t.TempDir()
	b1, created, err := certs.LoadOrCreate(dir, testenv.Domain)
	if err != nil || !created {
		t.Fatal(err)
	}
	b2, created, _ := certs.LoadOrCreate(dir, testenv.Domain)
	if created || b2.ThumbprintSHA256() != b1.ThumbprintSHA256() {
		t.Fatal("LoadOrCreate must reuse existing certificate")
	}
	b3, err := certs.Rotate(dir, testenv.Domain)
	if err != nil {
		t.Fatal(err)
	}
	if b3.ThumbprintSHA256() == b1.ThumbprintSHA256() {
		t.Fatal("rotate must issue a new server certificate")
	}
	if !b3.CA.Equal(b1.CA) {
		t.Fatal("rotate must keep the CA")
	}
	if _, _, err := certs.LoadOrCreate(dir, "other.test"); err == nil {
		t.Fatal("domain change must not silently replace the certificate")
	}
}

// T-TR-SCN-0111 / T-NFR-0401 / T-NFR-0504: 호출 기록은 Admin API로 조회되며 메타데이터만 담는다(본문·자격 증명 없음).
func TestCallTrace(t *testing.T) {
	e := testenv.Start(t, nil)
	_ = e.Login("7.0.3.0")
	res, err := e.AdminDo(http.MethodGet, "/v1/trace?limit=50", nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	var tr struct {
		Entries []map[string]any `json:"entries"`
	}
	_ = json.Unmarshal(raw, &tr)
	found := false
	for _, en := range tr.Entries {
		if en["soap_method"] == "Login" && en["session_version"] == "7.0.3.0" && en["correlation_id"] != "" && en["status"] == float64(200) {
			found = true
		}
	}
	if !found {
		t.Fatalf("Login not traced with metadata: %s", raw)
	}
	if strings.Contains(string(raw), e.User("vcenter").Password) {
		t.Fatal("trace must not contain credentials")
	}
}

// T-NFR-0405: 로그는 사람이 읽는 형식과 JSON 형식을 지원하고, 자격 증명을 남기지 않는다(T-NFR-0307).
// 추적: T-NFR-0401
func TestLogFormats(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	e := startWithLogger(t, log)
	_ = e.Login("")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var reqLine map[string]any
	for _, l := range lines {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) != nil {
			t.Fatalf("not JSON: %s", l)
		}
		if m["msg"] == "api request" && m["soap"] == "Login" {
			reqLine = m
		}
	}
	if reqLine == nil || reqLine["cid"] == "" {
		t.Fatalf("api request log missing: %s", buf.String())
	}
	if strings.Contains(buf.String(), e.User("vcenter").Password) {
		t.Fatal("log must not contain credentials")
	}
}

func startWithLogger(t *testing.T, log *slog.Logger) *testenv.Env {
	t.Helper()
	return testenv.StartWithLogger(t, nil, log)
}
