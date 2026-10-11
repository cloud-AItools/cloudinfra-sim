package sim

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"cloudinfra-sim/internal/rbac"
)

// User는 시뮬레이터 사용자다(FR-API-0210). Products는 접근 가능한 제품(vcenter, nsx)이다.
type User struct {
	Name     string   `json:"name"`
	Password string   `json:"password"`
	Products []string `json:"products"`
	Role     string   `json:"role,omitempty"`  // 역할(RBAC, FR-API-0211). 비면 전체 권한
	Scope    string   `json:"scope,omitempty"` // vCenter 권한 범위 엔터티 MoRef(비면 전체)
}

// Users는 사용자·Credential 저장소다. 코드에 기본 비밀번호를 두지 않고(FR-API-0214),
// 첫 기동 시 데이터 디렉터리에 무작위 비밀번호로 만든다(NFR-0304). 운영 자격 증명을 쓰지 않는다(NFR-0310).
type Users struct {
	mu   sync.Mutex
	list []User
	Path string
}

const usersFile = "users.json"

func LoadOrCreateUsers(dataDir string) (*Users, bool, error) {
	p := filepath.Join(dataDir, usersFile)
	b, err := os.ReadFile(p)
	if err == nil {
		var list []User
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, false, err
		}
		if len(list) == 0 {
			return nil, false, errors.New(usersFile + ": 사용자가 없다")
		}
		return &Users{list: list, Path: p}, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	list := []User{
		{Name: "administrator@vsphere.local", Password: randomPassword(), Products: []string{"vcenter"}},
		{Name: "admin", Password: randomPassword(), Products: []string{"nsx"}},
	}
	b, _ = json.MarshalIndent(list, "", "  ")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(p, b, 0o600); err != nil {
		return nil, false, err
	}
	return &Users{list: list, Path: p}, true, nil
}

func randomPassword() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b) + "!1aA"
}

// Check는 제품별 자격 증명을 상수 시간 비교로 확인한다.
func (u *Users) Check(product, name, password string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	ok := false
	for _, x := range u.list {
		if x.Name != name || !contains(x.Products, product) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(x.Password), []byte(password)) == 1 {
			ok = true
		}
	}
	return ok
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Role은 제품 사용자의 역할·범위다.
func (u *Users) Role(product, name string) (role, scope string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, x := range u.list {
		if x.Name == name && contains(x.Products, product) {
			return x.Role, x.Scope
		}
	}
	return "", ""
}

// Add는 시험용 사용자를 무작위 비밀번호로 만들어 저장한다(Admin API, FR-API-0213). 비밀번호를 돌려준다.
func (u *Users) Add(x User) (User, error) {
	if x.Name == "" || len(x.Products) != 1 {
		return User{}, errors.New("name과 products(vcenter 또는 nsx 하나)가 필요하다")
	}
	if !rbac.Valid(x.Products[0], x.Role) {
		return User{}, errors.New("role: vcenter=Admin|ReadOnly|VirtualMachineOperator, nsx=enterprise_admin|auditor")
	}
	if x.Scope != "" && x.Products[0] != "vcenter" {
		return User{}, errors.New("scope는 vcenter 사용자만")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, y := range u.list {
		if y.Name == x.Name && contains(y.Products, x.Products[0]) {
			return User{}, errors.New("이미 있는 사용자")
		}
	}
	x.Password = randomPassword()
	u.list = append(u.list, x)
	return x, u.saveLocked()
}

// Remove는 사용자를 지운다. 기본 사용자(역할 없음)는 지우지 않는다.
func (u *Users) Remove(name string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	for i, y := range u.list {
		if y.Name == name && y.Role != "" {
			u.list = append(u.list[:i], u.list[i+1:]...)
			_ = u.saveLocked()
			return true
		}
	}
	return false
}

// List는 비밀번호를 뺀 사용자 목록이다.
func (u *Users) List() []User {
	u.mu.Lock()
	defer u.mu.Unlock()
	out := make([]User, 0, len(u.list))
	for _, x := range u.list {
		x.Password = ""
		out = append(out, x)
	}
	return out
}

func (u *Users) saveLocked() error {
	b, _ := json.MarshalIndent(u.list, "", "  ")
	return os.WriteFile(u.Path, b, 0o600)
}

// Default는 제품의 기본 사용자(역할 제한 없음, 첫 기동 때 만든 계정)다. Scenario Runner가 쓴다(scenarios.go).
func (u *Users) Default(product string) (User, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	for _, x := range u.list {
		if x.Role == "" && contains(x.Products, product) {
			return x, true
		}
	}
	return User{}, false
}
