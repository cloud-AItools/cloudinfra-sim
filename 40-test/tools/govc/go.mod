// govc 빌드 전용 모듈(TR-COV-0305 범용 SDK 통합 시험). 시험 모듈(40-test)의 의존성과 분리한다.
// govc는 govmomi 저장소의 하위 모듈이며 별도 태그가 없으므로 govmomi v0.56.0 태그 커밋(81608f9b)으로 고정한다.
module cloudinfra-sim/test/tools/govc

go 1.27

require github.com/vmware/govmomi/govc v0.0.0-20260821034451-81608f9b9725

require (
	github.com/a8m/tree v0.0.0-20240104212747-2c8764a5f17e // indirect
	github.com/dougm/pretty v0.0.0-20171025230240-2ee9d7453c02 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/vmware/govmomi v0.56.0 // indirect
	github.com/xlab/treeprint v1.2.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
