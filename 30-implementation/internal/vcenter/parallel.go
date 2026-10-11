package vcenter

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// 파일 입출력 병렬화(R-45 ②): Windows에서는 파일 열기·닫기 한 번에 수 ms(보안 검사 포함)가 걸려 Snapshot·Fixture 캐시의
// 파일 수천 개를 하나씩 처리하면 지연이 파일 수에 비례한다. CPU가 아니라 지연이 병목이므로 동시에 처리한다.
// 순서와 무관한 작업(복사·읽기·쓰기)에만 쓴다 — MoRef 등 결정론에 영향을 주는 vcsim 객체 생성 순서는 바꾸지 않는다.

// IOWorkers는 동시 파일 작업 수다.
var IOWorkers = 16

// parallel은 items마다 fn을 IOWorkers개 고루틴으로 실행하고 첫 오류를 돌려준다.
func parallel[T any](items []T, fn func(T) error) error {
	ch := make(chan T)
	var wg sync.WaitGroup
	var once sync.Once
	var first error
	for range min(IOWorkers, max(len(items), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range ch {
				if err := fn(it); err != nil {
					once.Do(func() { first = err })
				}
			}
		}()
	}
	for _, it := range items {
		ch <- it
	}
	close(ch)
	wg.Wait()
	return first
}

// copyTree는 src 디렉터리 내용을 dst로 복사한다(dst가 없으면 만든다). 디렉터리를 먼저 만들고 파일은 병렬로 복사한다.
func copyTree(src, dst string) error {
	var files []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o700)
		}
		if !strings.HasSuffix(p, ".lck") {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return parallel(files, func(rel string) error { return copyFile(filepath.Join(src, rel), filepath.Join(dst, rel)) })
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// warmTree는 dir 아래 파일을 병렬로 한 번 읽는다. 순차로 읽는 vcsim Model.Load 전에 부르면 파일마다의 첫 열기 지연
// (OS 캐시·보안 검사)을 미리 치른다.
func warmTree(dir string) {
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	_ = parallel(files, func(p string) error {
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		_, _ = io.Copy(io.Discard, f)
		return f.Close()
	})
}

// 데이터스토어 묶음(R-45 ② 후속, 2026-10-11): Snapshot·Fixture 캐시는 데이터스토어 파일(VM마다 5개, 0 ~ 수백 바이트)을
// 파일 하나(.tar, 압축 없음)로 저장한다. 파일 수천 개를 만들고 해시하던 저장 시간이 파일 하나로 줄어든다.
// 원본은 병렬로 읽고 정렬한 순서로 쓴다(같은 내용이면 같은 묶음). 적재는 병렬로 푼다. 예전 디렉터리 형식도 읽는다.

const maxTarMem = 8 << 20 // 이보다 큰 파일은 미리 읽지 않고 쓸 때 읽는다

// tarTree는 src 디렉터리를 tarPath 하나로 묶는다(.lck 제외).
func tarTree(src, tarPath string) error {
	var files, dirs []string
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil || rel == "." {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, rel) // 빈 디렉터리도 보존
		} else if !strings.HasSuffix(p, ".lck") {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	data := make([][]byte, len(files))
	idx := make([]int, len(files))
	for i := range idx {
		idx[i] = i
	}
	if err := parallel(idx, func(i int) error {
		st, err := os.Stat(filepath.Join(src, files[i]))
		if err != nil || st.Size() > maxTarMem {
			return err
		}
		data[i], err = os.ReadFile(filepath.Join(src, files[i]))
		return err
	}); err != nil {
		return err
	}
	f, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	tw := tar.NewWriter(f)
	sort.Strings(dirs)
	for _, rel := range dirs {
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel) + "/", Mode: 0o700, Typeflag: tar.TypeDir}); err != nil {
			_ = f.Close()
			return err
		}
	}
	for i, rel := range files {
		b := data[i]
		if b == nil {
			if b, err = os.ReadFile(filepath.Join(src, rel)); err != nil {
				_ = f.Close()
				return err
			}
		}
		if err := tw.WriteHeader(&tar.Header{Name: filepath.ToSlash(rel), Mode: 0o600, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			_ = f.Close()
			return err
		}
		if _, err := tw.Write(b); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// untarTree는 tarPath를 dst 아래에 푼다(디렉터리는 먼저, 파일은 병렬로 쓴다). dst 밖을 가리키는 이름은 거부한다.
func untarTree(tarPath, dst string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	type entry struct {
		path string
		data []byte
	}
	var entries []entry
	dirs := map[string]bool{}
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		rel := filepath.FromSlash(strings.TrimSuffix(h.Name, "/"))
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("묶음 %s: 허용하지 않는 경로 %q", tarPath, h.Name)
		}
		if h.Typeflag == tar.TypeDir {
			dirs[filepath.Join(dst, rel)] = true
			continue
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		p := filepath.Join(dst, rel)
		dirs[filepath.Dir(p)] = true
		entries = append(entries, entry{p, b})
	}
	for d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	return parallel(entries, func(e entry) error { return os.WriteFile(e.path, e.data, 0o600) })
}
