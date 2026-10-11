package tempdir

import "syscall"

// processAlive: Windows의 os.FindProcess는 없는 pid에도 오류를 돌려주지 않는다(Go 1.27 확인) →
// 프로세스를 열어 종료 코드가 STILL_ACTIVE(259)인지 본다.
func processAlive(pid int) bool {
	const queryLimited = 0x1000 // PROCESS_QUERY_LIMITED_INFORMATION
	h, err := syscall.OpenProcess(queryLimited, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return true // 판단할 수 없으면 지우지 않는다
	}
	return code == 259
}
