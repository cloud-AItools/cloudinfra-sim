// Package buildinfo는 구현 식별자(소스 다이제스트)를 계산한다. 검증 증거를 구현에 묶는 데 쓴다(WP 1.13, FR-API-0302).
// Git 커밋을 쓸 수 있는 CI에서는 BUILD_COMMIT 환경변수가 우선한다.
package buildinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceDigest는 implDir 아래 Go 소스·go.mod·go.sum·프로파일 YAML의 SHA-256을 경로 순서대로 합산한다.
func SourceDigest(implDir string) (string, error) {
	if c := os.Getenv("BUILD_COMMIT"); c != "" {
		return c, nil
	}
	var files []string
	err := filepath.Walk(implDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(implDir, p)
		rel = filepath.ToSlash(rel)
		if strings.HasSuffix(rel, ".go") || rel == "go.mod" || rel == "go.sum" ||
			(strings.HasPrefix(rel, "data/compatibility-profiles/") && strings.HasSuffix(rel, "profile.yaml")) {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		f, err := os.Open(filepath.Join(implDir, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		io.WriteString(h, rel+"\x00")
		_, _ = io.Copy(h, f)
		f.Close()
	}
	return "src-" + hex.EncodeToString(h.Sum(nil))[:16], nil
}
