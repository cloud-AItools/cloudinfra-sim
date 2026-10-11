// Package tempdir는 프로세스 전용 임시 디렉터리를 만들고, 강제 종료로 남은 이전 프로세스의 것을 지운다.
//
// vcsim은 모델마다 데이터스토어 파일을 OS 임시 디렉터리(govcsim-*)에 만들고 정상 종료(Model.Remove) 때만 지운다.
// 작업 관리자 종료·kill -9·시험 Timeout처럼 강제로 끝나면 그대로 남아 쌓인다(2026-10-11: 136개, 407 MB).
// Isolate는 TMP·TEMP·TMPDIR를 <base>/<pid>로 바꿔 이후 임시 파일을 그 안에 모으고, 같은 base 아래 실행 중이 아닌
// pid의 디렉터리를 지운다. pid로 구분하므로 같은 base를 쓰는 다른 실행 중 프로세스의 것은 건드리지 않는다.
package tempdir

import (
	"os"
	"path/filepath"
	"strconv"
)

var vars = []string{"TMP", "TEMP", "TMPDIR"}

// Isolate는 <base>/<pid>를 만들고 임시 디렉터리 환경변수를 그곳으로 바꾼다. swept는 지운 이전 디렉터리,
// cleanup은 환경변수를 되돌리고 자기 디렉터리를 지운다(정상 종료 때 부른다).
func Isolate(base string) (dir string, swept []string, cleanup func(), err error) {
	swept = Sweep(base)
	dir = filepath.Join(base, strconv.Itoa(os.Getpid()))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", swept, func() {}, err
	}
	old := map[string]*string{}
	for _, k := range vars {
		if v, ok := os.LookupEnv(k); ok {
			old[k] = &v
		} else {
			old[k] = nil
		}
		_ = os.Setenv(k, dir)
	}
	return dir, swept, func() {
		for k, v := range old {
			if v == nil {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, *v)
			}
		}
		_ = os.RemoveAll(dir)
	}, nil
}

// Sweep는 base 아래에서 실행 중이 아닌 프로세스(이름 = pid)의 디렉터리를 지운다. 이름이 숫자가 아니거나
// 자기 자신·실행 중인 pid면 남긴다.
func Sweep(base string) (removed []string) {
	ents, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() || pid == os.Getpid() || processAlive(pid) {
			continue
		}
		p := filepath.Join(base, e.Name())
		if os.RemoveAll(p) == nil {
			removed = append(removed, p)
		}
	}
	return removed
}
