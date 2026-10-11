# cloudinfra-sim PRD 1.1 — CMP 요구기능 목록

> **목적:** 시뮬레이터 기능 범위를 선정할 때 참고할 **CMP 매뉴얼 기반 기능 후보 목록**을 제공한다.
> CMP는 참고 자료로만 활용한다. CMP 소스 분석·실제 CMP 연동 검증·CMP 상품 제약 재현은 범위 외이며, 이번 프로젝트는 시뮬레이션 구성과 자체 테스트까지 수행한다(PRD-4 D13). 150개 참고 기능은 구현 의무나 테스트 통과율의 분모가 아니다.

| 항목 | 내용 |
|---|---|
| 참고 CMP | Samsung Cloud Platform(SCP) for Enterprise 콘솔 |
| 출처 | Samsung Cloud Platform 사용자 가이드 v3.15 (SCP for Enterprise), `https://cloud.samsungsds.com/manual/ko/scp_user_guide.html` |
| 조회일 | 2026-10-10 (로그인 후 열람) |
| 이용 조건 | 로그인 후 열람한 자료를 요약·재구성한 내부 참고 문서다. 저장소를 외부에 공유·공개하기 전에 출처의 이용 조건을 확인하고, 필요하면 이 문서를 공개 범위에서 제외한다. |
| 대상 제품 버전 | vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x (Policy API 기반, NSX 4.x 전용 기능 미사용) — PRD-4 D1 (2026-10-10) |
| 작성 범위 | VMware vSphere·NSX 동작에 의존하는 Compute·Storage·Networking 상품의 기능 |
| 작성 방식 | 매뉴얼의 콘솔 기능을 요약·재구성한 것이다. 매뉴얼은 화면 동작을 설명할 뿐, 백엔드 구현은 공개하지 않는다. |

---

## 0. 작성 기준

### 0.1 ID 규칙

`CMP-<영역>-<번호>` 형식을 사용한다. 영역 코드는 다음과 같다.

| 코드 | 상품 / 기능 영역 | 코드 | 상품 / 기능 영역 |
|---|---|---|---|
| COM | 공통 (인증·인벤토리·작업 추적·태그) | IGW | VPC > Internet Gateway |
| VS | Virtual Server | NAT | VPC > NAT Gateway |
| BS | Block Storage (VM) | PEER | VPC > VPC Peering |
| GPU | GPU Server | RT | VPC > Routing Table |
| AS | Auto-Scaling | EP | VPC > Endpoint |
| VPC | VPC 본체 (로그·DNS 포함) | EDGE | VPC > Dedicated Edge Host |
| SUB | VPC > 서브넷·Virtual IP | SG | Security Group |
| PIP | VPC > Public IP | FW | Firewall |
| LB / LBS / LBG / LBP | Load Balancer / LB 서비스 / LB 서버그룹 / LB 프로파일 | TGW | Transit Gateway |
| DC | Direct Connect | VPN | VPN |
| DR | Virtual Server DR | BK | Backup |

### 0.2 범위 구분

| 구분 | 의미 |
|---|---|
| **핵심** | vCenter·NSX와 연관성이 높은 참고 기능. 우선 검토 후보이며 MVP 자동 포함을 뜻하지 않음 |
| **확장** | VMware·NSX에 의존하지만 MVP 이후 단계(Phase 3~4b)에서 다룰 후보 |
| **범위 외 후보** | 기존 분류명 보존. vSphere Replication·SRM·백업 솔루션·물리 회선·CMP 내부 기능은 현재 제외. 향후 확대하려면 별도 범위 변경 필요 |

`호환성 목표범위` 문서의 참고 계층은 이 구분을 다시 나눈 것이다. 계층 A(29개)는 '핵심' 중 같은 문서 §3.1에 나열한 기본 생명주기·디스크·네트워크 기능, 계층 B(45개)는 나머지 '핵심', 계층 C(58개)는 '확장'이며, '범위 외 후보'(18개)는 같은 이름으로 유지한다.

아래 표의 **상품 제약·한도, 신청/승인 절차, 과금, 콘솔 상태값은 모두 참고·범위 외**이다. 같은 수치나 용어가 VMware API에 있더라도 CMP 매뉴얼만으로 시뮬레이터의 제약으로 채택하지 않는다. 공식 API 계약의 독립 근거가 있는 제약만 승인된 API 범위에서 재현한다. §17의 API 매핑은 구현 후보이며 실제 SCP 호출 확인 결과가 아니다. 실제 채택 범위·선행 요구사항은 `호환성 목표범위` 문서와 PRD-4 D2·D7·D9에 기록한다.

### 0.3 대상 상품 선정

| 상품 | 분류 | VMware·NSX 연관 근거 (매뉴얼) | 범위 |
|---|---|---|---|
| Virtual Server | Compute | VMware Tools 옵션(`deployPkg enable-custom-scripts`) 적용 안내, VMware vSphere 환경 OVA 마이그레이션, 가상 하드웨어 버전 19 이하 제약 | 핵심 |
| Block Storage (VM) | Storage | VM에 직접 할당하는 디스크, 서버당 최대 15개, Multi Attach | 핵심 |
| GPU Server | Compute | VM 기반 GPU 할당, MIG·NVSwitch | 확장 |
| Auto-Scaling | Compute | Virtual Server·LB 서버그룹 자동 구성 | 확장 |
| VPC (서브넷, Public IP, IGW, NAT GW, Peering, Routing Table, Endpoint, Dedicated Edge Host) | Networking | 가상 사설 네트워크, NAT 로그 형식(SNAT/DNAT), Edge 전용 호스트 | 핵심 (Endpoint·Edge Host는 확장) |
| Security Group | Networking | 서버(vNIC) 단위 가상 방화벽, DFW 형식 로그(`INET match PASS/DROP`) | 핵심 |
| Firewall | Networking | IGW·TGW·Peering·DC·LB 단위 게이트웨이 방화벽, 규칙 순서·활성화 | 핵심 |
| Load Balancer | Networking | 크기 SMALL/MEDIUM/LARGE, 부하분산 알고리즘·멤버 상태·헤더 크기 기본값(1024/4096 byte)이 NSX LB 모델과 일치 | 핵심 |
| Transit Gateway | Networking | 다수 VPC 연결, BGP 업링크, 라우팅 테이블 | 확장 |
| Direct Connect | Networking | 1 VPC 연결, BGP 업링크 | 확장 |
| VPN | Networking | IPsec VPN 게이트웨이·터널 | 확장 |
| Virtual Server DR | Compute | 동기화 주기 5분~24시간, 복구 시점 최대 24개, 모의 훈련·재해 복구 (vSphere Replication·SRM 패턴) | 범위 외 후보 |
| Backup (VM Image) | Storage | VM 이미지 Full/Incremental 백업·복구 (스냅샷·CBT 기반 백업 솔루션 패턴) | 범위 외 후보 |

**제외한 상품:** Bare Metal Server, Block Storage (BM), File Storage, Object Storage, Archive Storage, DBaaS 전 상품, Kubernetes Engine(노드 VM은 Virtual Server 기능으로 포함), DNS(상품)·GSLB·Global CDN, Cloud WAN·Cloud LAN, Security 상품군(WAF, DDoS, IPS, Secured Firewall 등). VMware·NSX 동작과 직접 관련이 없거나, 관련 근거를 매뉴얼에서 찾지 못했다.

---

## 1. 공통 기능 (매뉴얼 공통 패턴에서 도출)

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-COM-01 | vCenter·NSX 연결·인증 | CMP가 관리 대상 vCenter·NSX Manager에 세션을 맺고 유지 | — | — | 핵심 |
| CMP-COM-02 | 인벤토리 조회 | 위치(서비스 존)·AZ, 클러스터·호스트, 데이터스토어(SSD/HDD), 네트워크 조회 (신청 화면의 선택 목록) | Multi-AZ 지원 위치만 AZ 선택 가능 | — | 핵심 |
| CMP-COM-03 | 비동기 작업 상태 추적 | 생성·수정·삭제 요청 후 완료까지 상태를 갱신 | 생성에 수십 분 걸릴 수 있음 | Creating → Active/Running, Editing, Deleting, Error/Unknown | 핵심 |
| CMP-COM-04 | 네트워크 설정 반영 확인 | 네트워크·보안 설정이 실제 반영될 때까지 대기 (예: SG 연결 약 5분) | — | Editing → Active, Error | 핵심 |
| CMP-COM-05 | 작업 이력 | 상품별 작업 이력 탭 (작업일시·결과·경로·작업자 검색) | — | — | 범위 외 후보 (CMP 내부) |
| CMP-COM-06 | 태그 | 모든 자원에 태그 추가·수정·삭제 | 자원당 최대 50개 | — | 확장 |

## 2. Virtual Server

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-VS-01 | 이미지 목록 조회 | 표준 / Custom / Migration / etc(DSC) / Kubernetes 이미지와 버전 선택 | 지원 OS: RHEL 8.10·9.4·9.6, Ubuntu 22.04·24.04, Windows 2016·2019·2022 Std, Alma·Rocky·Oracle Linux 8.x·9.6 | — | 핵심 |
| CMP-VS-02 | VM 생성 (사양·수량) | 서버 수 일괄 생성, 서버 타입(Standard / High Capacity 24vCore 이상) 선택, OS 디스크 용량, 추가 디스크(SSD/HDD), 암호화 | OS 디스크 100~300 GB, Custom Image 사용 시 서버 유형·암호화는 원본과 동일 | Creating → Running, Unknown | 핵심 |
| CMP-VS-03 | VM 생성 (네트워크) | VPC·일반 서브넷·IP(자동/직접 입력), NAT IP(자동/Public IP 선택), 로컬 서브넷 NIC 추가, DNS 사용 여부, Security Group 지정 | SG 최대 5개, NAT IP는 Public 서브넷+IGW 연결 시만, 서버별 설정은 10대 이하(11대 이상은 공통 설정만) | — | 핵심 |
| CMP-VS-04 | VM 생성 (게스트 초기화) | 서버명(Prefix-001…), Key pair 주입, Windows RDP 비밀번호, Init Script(Linux Shell / Windows PowerShell), 표준 NTP, 관리 계정 `sdscmpif` 생성 | `sdscmpif` 삭제 시 백업·복구 등 기능 불가 | — | 핵심 |
| CMP-VS-05 | VM 생성 (배치 정책) | AZ 지정, Placement Group 지정, Anti-affinity(서버 그룹 신규/기존) 적용 | Anti-affinity 서버 그룹은 5대 이하 | — | 확장 |
| CMP-VS-06 | 삭제 보호 (Deletion protection) | 사용 시 해지 요청을 거부 | — | — | 범위 외 후보 (CMP 내부) |
| CMP-VS-07 | VM 상세 조회 | 상태, 이미지·버전, 서버 타입(vCPU·메모리), 네트워크(VPC·서브넷·IP·NAT IP), 로컬 서브넷, AZ, SG, 디스크 목록, Init Script, 서버 그룹·Anti-affinity | — | Creating, Running, Stopped, Unknown | 핵심 |
| CMP-VS-08 | 시작 | 중지된 VM 가동 | — | Stopped → Running | 핵심 |
| CMP-VS-09 | 중지 | VM 강제 종료 | — | Running → Stopped | 핵심 |
| CMP-VS-10 | 재시작 | VM 다시 시작 | — | Running | 핵심 |
| CMP-VS-11 | 다중 VM 동시 제어 | 목록에서 여러 VM을 선택해 시작·중지·재시작 일괄 실행 | — | — | 핵심 |
| CMP-VS-12 | 서버 타입 변경 | vCPU·메모리 사양 변경, 변경 시 순차 재기동(Rolling) | 약정 기간 설정 시 축소 불가 | Running (재기동) | 핵심 |
| CMP-VS-13 | 네트워크 IP 변경 | 서브넷 선택 후 IP 변경 | VM Running 상태에서만 가능 | IP 옆 Editing / Error | 핵심 |
| CMP-VS-14 | NAT IP 설정·해제 | VM IP에 공인 NAT IP 1:1 매핑(자동 생성 또는 Public IP 선택) | Public 서브넷 + IGW 연결 필요 | Editing / Error | 핵심 |
| CMP-VS-15 | 로컬 서브넷 NIC 추가 | 서버 간 통신 전용 로컬 서브넷 연결 추가 | 로컬 서브넷은 외부 통신 불가 | — | 확장 |
| CMP-VS-16 | Security Group 연결·해제 | VM에 SG 추가·삭제 | 1~5개, 연결 반영 약 5분 | — | 핵심 |
| CMP-VS-17 | 디스크 증설·추가 | OS 디스크·추가 디스크 용량 증설, Block Storage(VM) 추가 신청으로 연결 | 축소 불가, OS 100~300 GB, 추가 디스크 최대 12,288 GB, Ubuntu는 재시작해야 인식 | — | 핵심 |
| CMP-VS-18 | 서버 그룹 정보 변경 | 서버 그룹명, Anti-affinity 사용 여부 변경 (그룹 내 모든 서버에 적용) | Anti-affinity는 5대 이하 그룹만 | — | 확장 |
| CMP-VS-19 | VM 해지 | VM 삭제 | 삭제 보호 사용 중이거나 다른 자원이 사용 중이면 불가 | Deleting | 핵심 |
| CMP-VS-20 | Custom Image 생성 | 중지된 VM으로 이미지 생성, 이름·설명 입력 | VM 중지 필요, 연결 디스크 총량 16 TB 이하 | Creating → Active | 핵심 |
| CMP-VS-21 | Custom Image 조회 | 이미지 정보, 사용 디스크 정보, 복사 신청 현황 | — | Active 등 | 핵심 |
| CMP-VS-22 | Custom Image 복사 | 다른 프로젝트로 복사 신청 → 대상 프로젝트에서 승인 후 생성 | 동일 서비스 존 내에서만 | — | 확장 |
| CMP-VS-23 | Custom Image 삭제 | 이미지 삭제 | — | — | 핵심 |
| CMP-VS-24 | Migration Image 생성 | OVA 파일 URL(+Access/Secret Key), OS Type, AZ, 원본 Root 계정으로 이미지 생성. 기존 NIC 제거 후 서버용 vNIC 추가 | 가상 HW 버전 19 이하, OS 디스크 SCSI 0:0·200 GB 이하, OVA·디스크 합계 1 TB 이하, 볼륨 8개 미만, 디스크 암호화·Passthrough·RDM 불가, CD-ROM·USB 연결 해제 | Creating → Active / 실패 | 확장 |
| CMP-VS-25 | Migration Image 조회·삭제 | 상세 조회, 실패 이미지 삭제 후 재생성 | — | — | 확장 |
| CMP-VS-26 | Placement Group 관리 | 그룹 생성(이름, VS 타입 Standard/High Capacity, AZ), 상세(대상 서버의 Rack·Block Storage 분산 적용 현황), 해지 | Best-Effort 분산, 최대 3개 분산, 생성 후 필수 정보 변경 불가, 적용 VM이 남으면 해지 불가 | — | 확장 |
| CMP-VS-27 | IP 일괄 변경 예약 | 대상 서버·변경 일시(30분 단위)·시간대 지정, 상세(대상 서버·스케줄 이력), 예약 해지 | 예약된 IP는 다른 자원이 사용 불가, VMware Tools·perl 최신 버전 필요 | — | 확장 |
| CMP-VS-28 | VM Metadata(IMDS) | VM 내부에서 토큰 발급 후 VM ID·IP·상태·VPC·SG 목록 등 조회 | 토큰 TTL 최대 21,600초 | — | 범위 외 후보 (CMP 서비스) |
| CMP-VS-29 | Linux 추가 스크립트 적용 | 2023년 3월 이전 Linux VM에 `vmware-toolbox-cmd config set deployPkg enable-custom-scripts true` 적용 (AWX 경유) | VM Running 상태 필요 | — | 확장 |

## 3. Block Storage (VM)

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-BS-01 | 디스크 생성·연결 | 이름, AZ, 적용 서버, 유형(SSD/HDD), 용량, 암호화, Multi Attach 여부 | 4~12,288 GB, 서버당 최대 15개, Multi Attach는 최초 신청 시만·Linux·SSD만·암호화/백업 불가, 암호화 시 약 10% 성능 저하·사후 해제 불가 | Creating → Active, Unknown | 핵심 |
| CMP-BS-02 | 디스크 상세 조회 | 디스크 정보·연결 서버 | OS 기본 디스크·복제본 디스크는 해지 버튼 없음 | Creating, Active, Resizing, Editing(Multi Attach 연결/해제), Deleting, Unknown | 핵심 |
| CMP-BS-03 | 용량 증설 | 추가 디스크 용량 증설 | 축소 불가, 최대 12,288 GB, Multi Attach·복제본 디스크 불가 | Resizing → Active | 핵심 |
| CMP-BS-04 | 적용 서버 수정 | Multi Attach 디스크에 서버 연결 추가·해제 | Multi Attach 디스크만 | Editing → Active | 확장 |
| CMP-BS-05 | 디스크 해지 | 디스크 삭제 | 모든 적용 서버 해제 후 가능, 복제본 디스크 불가 | Deleting | 핵심 |

## 4. GPU Server

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-GPU-01 | GPU VM 생성 | Virtual Server 생성 절차 + GPU 사양 선택, Tesla 드라이버 설치 이미지 | 지원 OS: RHEL 7.6·8.4·8.8, Ubuntu 18.04·20.04·22.04, Windows 2016 | Creating → Running | 확장 |
| CMP-GPU-02 | 상세·제어·자원 관리 | 시작·중지·재시작, IP 변경, 로컬 서브넷 추가, SG, 디스크 (VS와 동일) | — | VS와 동일 | 확장 |
| CMP-GPU-03 | Custom Image 관리 | 생성·조회·복사·삭제 (VS와 동일) | — | — | 확장 |
| CMP-GPU-04 | MIG(멀티 인스턴스 GPU) | NVIDIA A100 MIG 상품 신청 후 VM에서 MIG 적용·해제 | A100 한정 | — | 확장 |
| CMP-GPU-05 | NVSwitch 멀티 GPU | NVSwitch 상품 신청, VM 내 Fabric Manager 활성화 | — | — | 확장 |

## 5. Auto-Scaling

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-AS-01 | Launch Configuration 관리 | 이미지·서버 타입 등 VM 생성 템플릿 생성·조회·삭제 | — | — | 확장 |
| CMP-AS-02 | Auto-Scaling Group 생성 | 그룹명, 서버명, Min/Desired/Max, VPC·서브넷·NAT IP·로컬 서브넷, Multi-AZ, SG, LB+URL 패턴, Draining Timeout, File Storage | Max 50 | — | 확장 |
| CMP-AS-03 | 스케일링 정책 | 자원 상태 조건·연산자, 증감 방식·수, 쿨다운 | 그룹 내 정책명 중복 불가 | — | 확장 |
| CMP-AS-04 | 스케줄 스케일링 | 스케줄명, 서버 수, 주기, 시작·종료일, 시간대 | — | — | 확장 |
| CMP-AS-05 | LB 서버그룹 자동 연동 | ASG의 VM으로 LB 서버그룹(`ASG_xxxxxx`)을 자동 생성·갱신, LB 미사용 전환 시 자동 삭제 | 자동 생성 그룹은 사용자 삭제 불가 | — | 확장 |
| CMP-AS-06 | 이벤트·알림 | 스케일링 이벤트 조회, 알림 통보 방법·시점·수신인 | — | — | 범위 외 후보 (CMP 내부) |
| CMP-AS-07 | Auto-Scaling 해지 | ASG와 소속 VM 삭제 | — | — | 확장 |

## 6. VPC

### 6.1 VPC 본체

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-VPC-01 | VPC 생성 | 위치, VPC명(중복 체크), DNS Service IP(자동) | 서비스 존당 최대 5개 | Creating → Active | 핵심 |
| CMP-VPC-02 | VPC 상세·연결 자원 조회 | IGW, VPC Peering(요청 취소·승인·거부), TGW 연결(승인·거부), Direct Connect, 서브넷 목록 | — | Creating, Active, Deleting, Error | 핵심 |
| CMP-VPC-03 | VPC 해지 | VPC 삭제 | 서브넷, IGW·NAT GW, SG, Peering·TGW·DC 연결, 로그 저장소, ASG가 남아 있으면 불가 | Deleting | 핵심 |
| CMP-VPC-04 | 로그 저장소 관리 | Firewall(IGW/TGW/Peering/DC/LB), SG, NAT(SNAT·DNAT), LB 액세스 로그의 저장 위치·대상 지정 | Object Storage 과금 | — | 확장 |
| CMP-VPC-05 | 사용자 정의 DNS 등록 | 도메인, Source IP(서브넷·IP), DNS 서버 IP로 추가 DNS 등록·삭제 | VPC당 최대 3개, 고객사 사내 DNS만 허용 | — | 확장 |

### 6.2 서브넷·Virtual IP

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-SUB-01 | 서브넷 생성 | VPC, 용도(일반 Public/Private, 로컬 VM/BM), 이름, IP 대역(CIDR 중복 체크), Gateway(자동 지정) | VPC 내 다른 서브넷·LB IP·라우팅 목적지·Endpoint IP·업링크 Static Route와 중복 불가, 상위 CIDR 불가, 일부 관리용 대역 불가 | Creating → Active | 핵심 |
| CMP-SUB-02 | 서브넷 상세 조회 | 상세 정보, 사용 중 IP 목록 | — | Creating, Active, Editing, Terminating, Deleting, Failed, Error | 핵심 |
| CMP-SUB-03 | 서브넷 NAT Gateway 사용 전환 | 서브넷 상세에서 NAT GW 사용 설정 | IGW 연결 선행 필요 | Editing → Active | 핵심 |
| CMP-SUB-04 | Virtual IP 관리 | 서브넷 IP 예약·반납, VIP에 NAT IP 설정, VIP에 SG 적용 | — | — | 확장 |
| CMP-SUB-05 | 서브넷 삭제 | 서브넷 삭제 | NAT IP·NAT GW, VIP·연결 IP, Kubernetes Engine, VPC DNS 설정, ASG가 남아 있으면 불가 | Deleting | 핵심 |

### 6.3 Public IP

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-PIP-01 | Public IP 예약 | 위치, 구분(Internet Gateway / Secure Internet Gateway), 설명 | 공용·Secured IGW가 같은 IP Pool 사용 | Reserved | 핵심 |
| CMP-PIP-02 | Public IP 조회 | 할당 IP, 연결 자원 | — | Attached, Reserved | 핵심 |
| CMP-PIP-03 | Public IP 반납 | 예약 IP 반납 | Reserved 상태에서만 | — | 핵심 |

### 6.4 Internet Gateway

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-IGW-01 | IGW 생성 | VPC, 구분(공용 / 전용 / Secure), 이름 자동(`IGW_{VPC명}`, `DIGW_{VPC명}`), Firewall 사용·로깅 | 전용 IGW 매핑 이후에는 전용 IGW만 연결 가능 | Attaching → Attached | 핵심 |
| CMP-IGW-02 | 기본 경로 자동 생성 | IGW 생성 시 VPC Routing Table에 `0.0.0.0/0` 자동 추가, IGW Firewall 생성 | 기본 경로는 삭제 불가 | — | 핵심 |
| CMP-IGW-03 | IGW 상세 조회 | VPC, Firewall, NAT Gateway 연결 정보 | — | Attaching, Attached, Detaching, Detached, Terminating, Deleting, Error | 핵심 |
| CMP-IGW-04 | IGW 연결·연결 해제 | VPC와 연결 또는 해제 | — | Detaching → Detached | 핵심 |
| CMP-IGW-05 | IGW 삭제 | IGW 삭제 | 방화벽 규칙, NAT GW, 1:1 NAT IP, NAT 로깅 사용 설정이 남아 있으면 불가 | Deleting | 핵심 |

### 6.5 NAT Gateway

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-NAT-01 | NAT GW 생성 | VPC, 서브넷, NAT용 IP(자동 / 예약 Public IP) — 서브넷 전체 Outbound를 대표 공인 IP 1개로 변환 | IGW 연결 선행, 외부→VM 인바운드 불가 | Creating → Active | 핵심 |
| CMP-NAT-02 | NAT GW 조회 | VPC·서브넷 정보 | — | Creating, Active, Deleting, Deleted | 핵심 |
| CMP-NAT-03 | NAT GW 삭제 | 삭제 시 1:1 NAT 자원 외 인터넷 통신 불가 | — | Deleting → Deleted | 핵심 |

### 6.6 VPC Peering

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-PEER-01 | Peering 생성 요청 | 요청 VPC, 승인 프로젝트(동일/다른 프로젝트 ID), 승인 VPC, Firewall·로깅 | VPC당 최대 5개 | Requesting | 확장 |
| CMP-PEER-02 | 승인·거부·요청 취소 | 승인 측 VPC에서 승인(Firewall·로깅 설정) 또는 거부, 요청 측 취소 | — | Requesting → Active / Rejected / Canceled | 확장 |
| CMP-PEER-03 | Peering 조회 | 요청·승인 VPC, Firewall, Routing Table | — | Processing, Creating, Active, Requesting, Rejected, Canceled, Deleting, Failed, Error | 확장 |
| CMP-PEER-04 | Peering 삭제 | 삭제 | 양쪽 VPC의 관련 방화벽 규칙·라우팅 규칙 삭제 선행 | Deleting | 확장 |

### 6.7 Routing Table (VPC)

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-RT-01 | Routing Table 조회 | VPC 생성 시 자동 생성, 규칙 목록 | — | — | 핵심 |
| CMP-RT-02 | 라우팅 규칙 추가 | 목적지 IP(중복 체크), 경로(TGW-VPC 연결 / DCon-VPC 연결 / VPC Peering) | VPC 서브넷·LB IP·Endpoint IP와 중복·하위 대역 불가, 기존 목적지·관리 대역과 중복 불가 | — | 확장 |
| CMP-RT-03 | 라우팅 규칙 삭제 | 단건·다건 삭제 | IGW 기본 경로 `0.0.0.0/0` 삭제 불가 | — | 확장 |

### 6.8 Endpoint · Dedicated Edge Host

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-EP-01 | Endpoint 생성 | VPC, 대상 서비스(Object Storage / File Storage(New) / VPC DNS / Container Registry), 연결 자원, Endpoint IP | Endpoint IP는 서브넷·LB·다른 Endpoint·Routing Table·고객망 대역과 중복 불가, 외부망 접근 시 라우팅·방화벽 추가 설정 필요 | Creating → Active | 확장 |
| CMP-EP-02 | Endpoint 조회·삭제 | 상세 조회, 삭제 | 대상 서비스의 접근 제어 설정이 남아 있으면 삭제 불가 | Deleting → Deleted | 확장 |
| CMP-EDGE-01 | Dedicated Edge Host 신청 | 용도(VPC, IGW, LB, TGW, DiCon, L2B, STRG), 용량, 수량, 약정기간 | 수량 1~20, 장비 구축 완료 후 신청 | Active, Error | 확장 |
| CMP-EDGE-02 | Dedicated Edge Host 조회·해지 | 상세 조회, 해지 | — | — | 확장 |

## 7. Security Group

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-SG-01 | SG 생성 | 위치, 이름(VPC 내 중복 불가), VPC, 로깅 여부, 설명 | 서비스 존당 최대 100개, 로깅 사용 시 VPC 로그 저장소 선행 | Creating → Active | 핵심 |
| CMP-SG-02 | SG 상세 조회 | 상세 정보, 적용 대상, Inbound/Outbound 규칙 | — | Creating, Active, Editing, Error, Terminating | 핵심 |
| CMP-SG-03 | 기본 차단 | 규칙에 없는 트래픽은 시스템 생성 default any-any deny 규칙으로 차단. 같은 SG 서버 간 통신도 IP 규칙 등록 필요 | — | — | 핵심 |
| CMP-SG-04 | 규칙 추가 | 방향(Inbound/Outbound), 대상 주소(CIDR·범위·단일, 최대 128개), 프로토콜(ALL 포함), 허용 포트(목록/직접 입력), 설명, 기존 규칙 복사 | 서비스 존당 규칙 1,000개 | 규칙 상태 (Error 등) | 핵심 |
| CMP-SG-05 | 규칙 일괄 추가 | Excel 양식으로 규칙 업로드 | 1회 최대 100개 | — | 확장 |
| CMP-SG-06 | 규칙 조회·검색·수정 | 대상 주소·설명·상태로 검색, 규칙 수정, Excel 다운로드 | — | — | 핵심 |
| CMP-SG-07 | 규칙 삭제 | 규칙 삭제 | Error 상태 규칙, 다른 상품이 자동 생성한 규칙은 삭제 불가 | — | 핵심 |
| CMP-SG-08 | 멀티캐스트 대상 추가 | 적용 대상에 멀티캐스트 IP 직접 추가·삭제 | 224.0.0.0~239.255.255.255 | — | 확장 |
| CMP-SG-09 | 로깅 설정 변경 | 로그 저장 사용·해제 | VPC 로그 저장소 지정 필요 | — | 확장 |
| CMP-SG-10 | SG 해지 | SG 삭제 | 로깅 사용 중이거나 적용 자원이 있으면 불가 | Terminating | 핵심 |
| CMP-SG-11 | SG 로그 | `INET match PASS / DROP`, `INET TERM (PASS)` 로그(Rule-ID, 방향, 프로토콜, 종료 사유 FIN/RST/TIMEOUT, IP/Port) | — | — | 확장 |

## 8. Firewall

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-FW-01 | Firewall 생성 | IGW, TGW-VPC 연결, VPC Peering, DCon-VPC 연결, Load Balancer 생성 시 'Firewall 사용'으로 함께 생성 | 연계 상품별 1개 | Active | 핵심 |
| CMP-FW-02 | Firewall 상세 조회 | Firewall 구분, VPC, 연결명, 규칙 수, 로깅 여부·로그 저장소 | 서비스 존당 규칙 1,000개, Firewall당 최대 1,000개 | Active, InActive | 핵심 |
| CMP-FW-03 | 연결·연결 해제 | Firewall 적용·해제 | 해제 시 보안 노출 경고 | Active ↔ InActive | 핵심 |
| CMP-FW-04 | 규칙 추가 | 출발지·목적지 IP(각 최대 128개), 프로토콜, 허용 포트, 동작(Allow/Deny), 방향(Inbound/Outbound/In-Out), 규칙 위치(지정 규칙 앞·뒤 / 마지막), 설명, 복사 | VPC 내부 자원은 NAT IP가 아닌 Private IP로 지정 | — | 핵심 |
| CMP-FW-05 | 규칙 일괄 입력 | Excel 업로드 | 최대 100개, 비활성 상태로 등록됨 | — | 확장 |
| CMP-FW-06 | 규칙 수정·활성화·비활성화 | 규칙 내용·설명 수정, 활성/비활성 전환 | — | — | 핵심 |
| CMP-FW-07 | 규칙 순서 변경 | 기준 규칙 ID 앞·뒤로 이동 | — | — | 핵심 |
| CMP-FW-08 | 규칙 삭제 | 단건·다건 삭제 | — | — | 핵심 |
| CMP-FW-09 | 로깅 설정 변경 | 로깅 사용 여부 변경 | VPC 로그 저장소 필요 | — | 확장 |
| CMP-FW-10 | Firewall 로그 | `INET reason-match PASS/DROP/RDR(NAT)`, `INET TERM PASS/RDR`, TCP Flag(A/P/R/S/F) | — | — | 확장 |

## 9. Load Balancer

### 9.1 Load Balancer

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-LB-01 | LB 생성 | 이름, VPC, 크기(SMALL/MEDIUM/LARGE), LB 서비스 IP 대역, LB Link IP 대역(서버 측 출발지 /30), Firewall·로깅 | 서비스 수 SMALL 20 / MEDIUM 100 / LARGE 1,000, VPC당 SMALL 1+MEDIUM 1, LARGE는 서비스 존당 1개이며 다른 LB와 공존 불가, Link IP는 192.168.254.0/30~.36/30 | Creating → Active | 핵심 |
| CMP-LB-02 | LB 상세·연결 자원 조회 | 상세, LB 서비스·서버그룹·프로파일 목록 | — | Creating, Active, Editing, Error, Terminating | 핵심 |
| CMP-LB-03 | LB 해지 | LB 삭제 | 하위 서비스·서버그룹·프로파일, LB Firewall 규칙, Kubernetes 사용이 남아 있으면 불가 | Terminating | 핵심 |

### 9.2 LB 서비스

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-LBS-01 | LB 서비스 생성 | 서비스 구분(L4 TCP / L4 UDP / L7 HTTP / L7 HTTPS), 서비스 IP(신규·기존·직접 입력), 서비스 포트, 전달 포트, NAT IP | 같은 IP에 포트 중복 불가, L4 포트는 단일 14개·범위 7개까지, 모니터링은 첫 포트만 | CREATING → UP 등 | 핵심 |
| CMP-LBS-02 | L7 URL 라우팅 | URL 시작 패턴별 서버그룹 지정, 순서 변경, 기본 패턴(`/`) | 첫 일치 규칙만 적용, 기본 패턴 삭제·이동 불가 | — | 핵심 |
| CMP-LBS-03 | 프로파일 지정 | 지속성(소스 IP / 쿠키), 애플리케이션 프로파일(기본값 자동 생성), L7 HTTP 리디렉션 | 동일 서버그룹 공유 시 지속성 옵션 제한, 리디렉션 프로파일 사용 시 서버그룹 미지정 | — | 핵심 |
| CMP-LBS-04 | SSL 설정 (HTTPS) | Certificate Manager 인증서로 클라이언트 SSL·서버 SSL 지정, 보안 수준(보통/높음) | TLS 1.2만 지원 | — | 확장 |
| CMP-LBS-05 | 액세스 로그 | 액세스 로그 저장 사용 | VPC 로그 저장소 필요 | — | 확장 |
| CMP-LBS-06 | LB 서비스 조회·수정 | 상태, 서비스 포트·전달 포트·NAT IP·서버그룹 수정 | — | UP, PARTIALLY_UP, DOWN, DETACHED, UNKNOWN, CREATING, EDITING, DELETING, FAILED, ERROR | 핵심 |
| CMP-LBS-07 | LB 서비스 삭제 | 삭제 | Auto-Scaling·Kubernetes 사용, 서버그룹 연결, 액세스 로그 사용 중이면 불가 | DELETING | 핵심 |

### 9.3 LB 서버그룹

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-LBG-01 | 서버그룹 생성 | 부하분산(Round robin / Weighted round robin / Least connection / Weighted least connection / IP hash), 대상 서버(같은 VPC VM 또는 IP 직접 입력), 멤버별 포트·가중치(1~256)·활성화(ENABLED / DISABLED / GRACEFULLY DISABLED) | 같은 서버를 여러 멤버로 등록 시 포트 필수·상이 | CREATING → ACTIVE | 핵심 |
| CMP-LBG-02 | 헬스체크 | 프로토콜, 포트, 주기, 대기 시간, 탐지 횟수, HTTP Method(GET/POST), URL, 전송·응답 문자열, HTTP 1.1 사용 | — | — | 핵심 |
| CMP-LBG-03 | 서버그룹 조회·수정 | 상세 조회, 항목 수정 | ASG 자동 생성 그룹은 부하분산·헬스체크·HTTP 1.1만 수정 가능 | CREATING, ACTIVE, EDITING, ERROR, TERMINATING | 핵심 |
| CMP-LBG-04 | 서버그룹 삭제 | 삭제 | LB 서비스 연결 중이면 불가, ASG 자동 생성 그룹은 직접 삭제 불가 | TERMINATING | 핵심 |

### 9.4 LB 프로파일

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-LBP-01 | 지속성 프로파일 | 소스 IP / 쿠키(쿠키 이름 = 프로파일명) | LB 내 이름 중복 불가 | — | 핵심 |
| CMP-LBP-02 | 애플리케이션 프로파일 | 서비스 구분(L4 TCP / L4 UDP / L7 HTTP), L7 HTTP 요청 헤더 크기(기본 1024 byte)·응답 헤더 크기(기본 4096 byte) 등 | 헤더 크기 0~65,536 byte | — | 핵심 |
| CMP-LBP-03 | 프로파일 조회·수정·삭제 | 상세 조회, 수정, 삭제 | 서비스가 참조 중이면 삭제 불가(추정) | — | 핵심 |

## 10. Transit Gateway

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-TGW-01 | TGW 생성 | 이름, Uplink 사용·포트 용량(1G/10G, 사전 신청 시 20G/40G) | 프로젝트당 최대 3개, 포트 용량 변경 불가 | Creating → Active | 확장 |
| CMP-TGW-02 | TGW-VPC 연결 | TGW, VPC, 연결명 자동, Firewall·로깅, VPC 측 승인 | TGW당 VPC 5개(동일 프로젝트), VPC당 TGW 3개 | Requesting → Active / Rejected | 확장 |
| CMP-TGW-03 | 업링크 회선 연결 | SR로 회선 신청·해지 (고객 라우터 BGP 필수, 약 10일) | Uplink와 TGW Peering 동시 사용 불가 | — | 범위 외 후보 (물리 회선) |
| CMP-TGW-04 | TGW Routing Table 관리 | 목적지 IP(VPC 서브넷 대역)·경로(TGW-VPC 연결) 규칙 추가·삭제 | — | — | 확장 |
| CMP-TGW-05 | TGW Peering | 다른 프로젝트 TGW와 Peering 요청·승인, 양측 라우팅 설정 | 업링크 연결 TGW는 별도 TGW 필요 | — | 확장 |
| CMP-TGW-06 | TGW 조회·해지·연결 삭제 | 상세·연결 자원(Uplink, TGW-VPC, Peering, Cloud WAN) 조회, 해지 | VPC 연결·Peering·Uplink가 남아 있으면 해지 불가, 연결 삭제 전 방화벽·라우팅 규칙 삭제 | Creating, Active, Inactive, Deleting, Deleted, Failed, Error | 확장 |

## 11. Direct Connect

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-DC-01 | Direct Connect 생성 | 이름, Uplink 포트 용량(1G/10G) | 포트 용량 변경 불가 | Creating → Active | 확장 |
| CMP-DC-02 | DCon-VPC 연결 | Direct Connect, VPC, 연결명 자동, Firewall·로깅 | Direct Connect당 VPC 1개 | — | 확장 |
| CMP-DC-03 | 회선 연결 | SR로 회선 신청 (고객 라우터 BGP 필수, 약 10일) | — | — | 범위 외 후보 (물리 회선) |
| CMP-DC-04 | DC Routing Table 관리 | 목적지 IP(VPC 서브넷)·경로(DCon-VPC 연결) 규칙 추가·삭제 | — | — | 확장 |
| CMP-DC-05 | 조회·해지·연결 삭제 | 상세·연결 정보 조회, 해지, DCon-VPC 연결 삭제 | — | — | 확장 |

## 12. VPN

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-VPN-01 | VPN Gateway 생성 | 이름, QoS 대역폭(10~100 Mbps, 10 단위, 양방향), Public IP(자동), Local Subnet(`x.x.x.0/24`, 최소 IP는 GW) | 프로젝트당 1개, Local Subnet은 원격망·VPC 서브넷과 중복 불가 | Creating → Active | 확장 |
| CMP-VPN-02 | VPN 조회·수정 | 상세, QoS 대역폭·설명 변경, Local Subnet 정보 | — | Creating, Active, Editing, Error, Terminating | 확장 |
| CMP-VPN-03 | VPN 터널 관리 | 터널 생성·조회·삭제 (원격 피어·IPsec 설정) | VPN당 최대 5개 | — | 확장 |
| CMP-VPN-04 | VPN 연결 관리 | Virtual Server를 VPN Local Subnet에 연결·해제, 게스트 OS 라우팅 설정 안내 | — | — | 확장 |
| CMP-VPN-05 | VPN 해지 | VPN 삭제 | — | Terminating | 확장 |

## 13. Virtual Server DR

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-DR-01 | DR Group 관리 | 용도·위치·VPC 기준 그룹 생성·조회·삭제 (DR 위치 지정) | — | — | 범위 외 후보 |
| CMP-DR-02 | Network Pairing 관리 | 원본·DR 서브넷(VPC) 매핑 생성·조회·삭제 | Replication 신청 전 설정 필수 | — | 범위 외 후보 |
| CMP-DR-03 | DR Replication 생성 | DR Group, 대상 VM(최대 10대), DR 측 IP(자동 / 원본과 동일 / 입력)·NAT IP·SG, 동기화 주기, 복구 시점(저장 주기·수량) | 동기화 5분~24시간, 저장 주기 1~24시간, 저장 수량 1~24개, Multi Attach VM·디스크 암호화 혼재 VM 불가, 적용 후 디스크 추가 불가 | — | 범위 외 후보 |
| CMP-DR-04 | DR Replication 조회·삭제 | 상세 조회, 삭제 | — | — | 범위 외 후보 |
| CMP-DR-05 | Recovery Plan 관리 | 복구 계획 생성·조회(서버별 설정·진행 현황)·삭제 | — | — | 범위 외 후보 |
| CMP-DR-06 | 모의 훈련 | 모의 훈련 시작, 훈련용 자원 정리 | — | — | 범위 외 후보 |
| CMP-DR-07 | 재해 복구 | 재해 복구 시작·완료 | — | — | 범위 외 후보 |

## 14. Backup (VM Image)

| ID | 기능 | 주요 입력·동작 | 상품 제약·한도 (참고·범위 외) | 콘솔 상태값 (참고·범위 외) | 범위 |
|---|---|---|---|---|---|
| CMP-BK-01 | VM Image 백업 생성 | 대상 VM(Running), 저장소(SD-Storage), Full 1개 + Incremental 최대 6개 스케줄, 보관기간, DR 복제(다른 리전) | Multi Attach VM은 Incremental 불가, Incremental 보관 ≤ Full 보관 | — | 범위 외 후보 |
| CMP-BK-02 | 백업 조회 | 스케줄, 수행 이력, 복구 대상, 복구 이력 | — | — | 범위 외 후보 |
| CMP-BK-03 | 백업 설정 변경 | 보관기간·스케줄 변경, 백업 파일 삭제, DR 해제 | — | — | 범위 외 후보 |
| CMP-BK-04 | VM Image 복구 | 일반 백업·DR 백업으로 VM 복구 | — | — | 범위 외 후보 |
| CMP-BK-05 | 백업 작업 취소·해지 | 진행 중 작업 취소, 백업 해지 | — | — | 범위 외 후보 |

---

## 15. 기능 수 집계

| 영역 | 핵심 | 확장 | 범위 외 후보 | 계 |
|---|---:|---:|---:|---:|
| 공통 (COM) | 4 | 1 | 1 | 6 |
| Virtual Server (VS) | 18 | 9 | 2 | 29 |
| Block Storage (BS) | 4 | 1 | 0 | 5 |
| GPU Server (GPU) | 0 | 5 | 0 | 5 |
| Auto-Scaling (AS) | 0 | 6 | 1 | 7 |
| VPC 본체·서브넷·Public IP·IGW·NAT (VPC, SUB, PIP, IGW, NAT) | 18 | 3 | 0 | 21 |
| VPC Peering·Routing Table·Endpoint·Edge (PEER, RT, EP, EDGE) | 1 | 10 | 0 | 11 |
| Security Group (SG) | 7 | 4 | 0 | 11 |
| Firewall (FW) | 7 | 3 | 0 | 10 |
| Load Balancer (LB, LBS, LBG, LBP) | 15 | 2 | 0 | 17 |
| Transit Gateway·Direct Connect (TGW, DC) | 0 | 9 | 2 | 11 |
| VPN | 0 | 5 | 0 | 5 |
| Virtual Server DR (DR) | 0 | 0 | 7 | 7 |
| Backup (BK) | 0 | 0 | 5 | 5 |
| **합계** | **74** | **58** | **18** | **150** |

## 16. 후속 작업

1. ~~각 기능 항목에 해당하는 vCenter·NSX API를 매핑한다.~~ → §17에 반영 (2026-10-10)
2. 프로젝트에서 시뮬레이션할 기능·API를 선정해 Phase 0 잠정 기준선으로 기록한다. CMP 운영 측 검토·소스 분석·실제 호출 확인은 요구하지 않는다.
3. §17 후보를 공식 API 계약·공개 SDK와 대조하고, 채택한 API의 정상·오류·상태 전이를 자체 테스트한다. 실제 CMP 경로는 미확인으로 남기며 외부 Trace·AC-20은 현재 범위에서 제외한다.
4. §18의 미정의 API는 후보 기능을 채택할 때만 FR·선행 WP·테스트를 함께 추가한다. 목록의 공백이 곧 구현 의무를 뜻하지 않는다.

---

## 17. vCenter·NSX API 매핑

### 17.1 매핑 기준

- **추정 매핑이다.** SCP 매뉴얼은 콘솔 동작만 설명하므로, 각 기능을 구현하려면 VMware 공식 API 중 무엇이 필요한지 추정했다. SCP CMP의 실제 호출 경로는 미확인이며 이번 범위에서 조사하지 않는다. 시뮬레이터에 채택할 경로는 공식 계약을 근거로 별도 선정한다.
- **표기:** `SOAP` = vSphere Web Services(`/sdk`) 메서드, `REST` = vSphere Automation API(`/api`, 구형 `/rest`는 같은 기능의 대응 경로), `PBM` = `/pbm/sdk`, `Policy` = NSX Policy API(`/policy/api/v1` 생략 표기), `MP` = NSX Manager API(`/api/v1` 생략 표기). `{t1}` 등은 경로 변수다.
- **SOAP과 REST를 함께 적은 경우** 후보 경로를 뜻한다. 공식 계약·자체 테스트 범위에 따라 채택하며, 두 경로를 지원하면 동일 객체의 상태 일관성을 검증한다(FR-VC-0510).
- **신뢰도:** 상 = 매뉴얼의 수치·열거값·로그 형식이 VMware/NSX 모델과 일치함 / 중 = 기능상 해당 API 외 대안이 거의 없음 / 하 = 여러 구현 방식이 가능해 경로 선정이 필요함. 신뢰도는 매핑 추정에 관한 것으로 API 구현·검증 상태와 다르다.
- **네트워크 토폴로지 가정:** VPC = NSX Tier-1 Gateway, 서브넷 = Segment, Security Group = DFW, Firewall = Gateway Firewall, Load Balancer = NSX LB로 가정했다. SCP의 'VPC'는 NSX 4.x의 NSX VPC(`/orgs/default/projects/{p}/vpcs/{vpc}`)가 아니다. 대상이 NSX-T 3.x이므로 모든 Policy 경로는 `/infra` 하위다(FR-NSX-0230 적용 제외 — D1).
- **대상 버전 반영 (D1):** vSphere/vCenter 7.0 U2 이상(8.x 포함), NSX-T 3.x(Policy API 기반, 4.x 전용 기능 미사용).
  - vSphere Automation `/api` 경로는 7.0 U2에서 도입되었으므로 대상 버전 전체에서 `/api`와 구형 `/rest`가 함께 존재한다. 채택한 각 경로를 자체 계약 테스트하며, 실제 CMP 경로 확인은 요구하지 않는다(FR-VC-0203, FR-VC-0204).
  - CMP-VS-24의 '가상 하드웨어 버전 19 이하' 제약은 vSphere 7.0 U2의 최대 하드웨어 버전(19)과 일치한다. vSphere 8.x(최대 HW 20·21)에서도 SCP는 19 이하만 허용하므로, 시뮬레이터는 공식 계약에 따른 버전별 HW 제약만 재현하며 SCP의 HW 19 제한은 적용하지 않는다.
  - vSphere 8.x는 NSX-T 3.2.x 일부 빌드(3.2.2 이상으로 파악, VMware 상호운용성 매트릭스로 확인 필요)와만 조합되므로, 8.x 기반 판정 조합의 NSX 버전은 이 범위로 제한된다.
  - NSX-T 3.0 / 3.1 / 3.2 사이의 필드·경로 차이는 버전 프로파일로 처리한다. NSX Manager API는 인증·버전·상태와 Policy API로 대체할 수 없는 채택 기능(Fabric VM·VIF·Edge 조회, 선택적 VM Tag 등)에 한정한다. 쓰기 기능 채택 여부는 D9에 기록한다.

**경로 정정 근거:** Content Library 파일 추가 경로는 `POST /api/content/library/item/update-session/{updateSessionId}/file`이다([Broadcom 공식 API 문서](https://developer.broadcom.com/xapis/vsphere-automation-api/latest/api/content/library/item/update-session/updateSessionId/file/post/)). CMP-VS-24에 반영했으며 채택 시 대상 버전 스펙으로 재확인한다.

### 17.2 공통

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-COM-01 | SOAP `RetrieveServiceContent`, `SessionManager.Login`/`LoginByToken`, `Logout` / REST `POST /api/session`, `DELETE /api/session` | `POST /api/session/create`(JSESSIONID·X-XSRF-TOKEN) 또는 Basic 인증, MP `GET /node/version` | 중 | CMP SDK(pyVmomi·govmomi 등)에 따라 SOAP/REST 세션 방식이 다름 | FR-VC-0201, 0203, 0209 / FR-NSX-0201, 0202 |
| CMP-COM-02 | SOAP `ViewManager.CreateContainerView` + `RetrievePropertiesEx`(Datacenter, ClusterComputeResource, HostSystem, Datastore, Network, ResourcePool) / REST `GET /api/vcenter/datacenter`, `/cluster`, `/host`, `/datastore`, `/network`, `/resource-pool` / PBM `PbmQueryProfile`, `PbmRetrieveContent` | Policy `GET /infra/sites/default/enforcement-points/default/edge-clusters`, `.../transport-zones` | 중 | AZ = 클러스터 또는 리소스 풀, SSD/HDD = 데이터스토어 유형 또는 스토리지 정책으로 추정 | FR-VC-0202, 0301~0306, 0210 / FR-NSX-0205, 0210 |
| CMP-COM-03 | SOAP Task MoRef + `WaitForUpdatesEx` 또는 `RetrievePropertiesEx`(`info.state`, `info.progress`, `info.error`) / REST 동기 응답 또는 `GET /api/cis/tasks/{task}` | — | 상 | Creating → Running/Active 전이는 Task 완료 시점에 대응 | FR-VC-0501~0509 |
| CMP-COM-04 | — | Policy `GET /infra/realized-state/status?intent_path=…`, `GET /infra/realized-state/realized-entities?intent_path=…` | 중 | SG 연결 '약 5분' 등 반영 대기는 Realization 폴링으로 추정 | FR-NSX-0220, 0501~0509 |
| CMP-COM-05 | (선택) SOAP `EventManager.QueryEvents`, `TaskManager.CreateCollectorForTasks` | — | 하 | CMP 자체 이력일 가능성이 높음 | FR-MET |
| CMP-COM-06 | (선택) REST `POST /api/cis/tagging/tag-association/{tag}?action=attach`, `…?action=detach` | (선택) Policy 리소스 `tags` 필드 | 하 | 콘솔 태그를 VMware 태그로 동기화하는지 불명 | FR-VC-0203 / FR-NSX-0219 |

### 17.3 Virtual Server

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-VS-01 | REST `GET /api/content/library`, `GET /api/content/library/item?library_id=`, `GET /api/vcenter/vm-template/library-items/{item}` / SOAP 템플릿 VM 조회(`config.template=true`) | — | 중 | 표준·Custom 이미지 = Content Library 항목 또는 vCenter 템플릿 | FR-VC-0206, 0305 |
| CMP-VS-02 | SOAP `CloneVM_Task`(VirtualMachineCloneSpec: location, config.numCPUs·memoryMB, powerOn) / REST `POST /api/vcenter/vm-template/library-items/{item}?action=deploy`, `POST /api/vcenter/ovf/library-item/{item}?action=deploy`, `POST /api/vcenter/vm?action=clone` / 추가 디스크·암호화: `VirtualDeviceConfigSpec`(VirtualDisk, `profile`=PBM 정책), `VirtualMachineConfigSpec.crypto` | — | 중 | 서버 수 N대 = Clone/Deploy N회. 암호화는 VM Encryption 스토리지 정책 + KMS 필요 | FR-VC-0401, 0402, 0206, 0210 |
| CMP-VS-03 | SOAP Clone/Reconfig의 `VirtualVmxnet3` 추가(backing: `VirtualEthernetCardOpaqueNetworkBackingInfo` 또는 `VirtualEthernetCardDistributedVirtualPortBackingInfo`) / REST `POST /api/vcenter/vm/{vm}/hardware/ethernet` | Policy `PATCH /infra/segments/{seg}`(대상 서브넷 확인), Segment Port 자동 생성 조회 `GET /infra/segments/{seg}/ports`, NAT IP는 CMP-VS-14, SG는 CMP-VS-16 | 중 | 로컬 서브넷 NIC = 두 번째 vNIC. NSX-backed 네트워크 표현은 버전별로 다름 | FR-VC-0306 / FR-NSX-0211, 0603, 0604, 0607 |
| CMP-VS-04 | SOAP `CloneVM_Task`의 `CustomizationSpec`(CustomizationLinuxPrep `hostName`·`scriptText` / CustomizationSysprep `guiRunOnce`·관리자 비밀번호, IP·DNS 설정) 또는 `CustomizeVM_Task` / REST `PUT /api/vcenter/vm/{vm}/guest/customization` | — | 중 | VS-29의 `enable-custom-scripts` 안내는 Linux 게스트 커스터마이징 스크립트 사용의 근거. Key pair·`sdscmpif` 계정은 스크립트로 주입한 것으로 추정 | FR-VC-0416 |
| CMP-VS-05 | SOAP `ReconfigureComputeResource_Task`(ClusterConfigSpecEx.rulesSpec: `ClusterAntiAffinityRuleSpec`; Placement Group은 `ClusterVmGroup`+`ClusterHostGroup`+`ClusterVmHostRuleInfo(mandatory=false)`), 클러스터·리소스 풀 지정 | — | 중 | 'Best-Effort' = 필수가 아닌(should) VM-Host 규칙. Rack 분산 = Rack별 Host Group으로 추정 | FR-VC-0302 (DRS 규칙 FR 없음) |
| CMP-VS-06 | 없음 | 없음 | 상 | CMP 내부 플래그 | — |
| CMP-VS-07 | SOAP `RetrievePropertiesEx`(runtime.powerState, config.hardware, guest.ipAddress, guest.net, guest.toolsRunningStatus, layoutEx) / REST `GET /api/vcenter/vm/{vm}`, `GET /api/vcenter/vm/{vm}/guest/identity`, `GET /api/vcenter/vm/{vm}/guest/networking/interfaces` | (선택) Policy `GET /infra/realized-state/enforcement-points/default/virtual-machines`, MP `GET /fabric/virtual-machines`, `GET /fabric/vifs` | 상 | Running/Stopped = poweredOn/poweredOff, Unknown = 생성 Task 실패 | FR-VC-0305, 0507 / FR-NSX-0226~0228 |
| CMP-VS-08 | SOAP `PowerOnVM_Task` / REST `POST /api/vcenter/vm/{vm}/power?action=start` | — | 상 | — | FR-VC-0405 |
| CMP-VS-09 | SOAP `PowerOffVM_Task` / REST `POST /api/vcenter/vm/{vm}/power?action=stop` | — | 상 | 매뉴얼이 '강제 종료'로 명시 → 게스트 종료(`ShutdownGuest`)가 아닌 전원 끄기 | FR-VC-0406 |
| CMP-VS-10 | SOAP `RebootGuest` 또는 `ResetVM_Task` / REST `POST /api/vcenter/vm/{vm}/guest/power?action=reboot` 또는 `POST /api/vcenter/vm/{vm}/power?action=reset` | — | 중 | 게스트 재부팅인지 하드 리셋인지 구현 경로 선정 필요(실제 CMP 경로 미확인) | FR-VC-0407 |
| CMP-VS-11 | SOAP `Datacenter.PowerOnMultiVM_Task` 또는 VM별 `PowerOnVM_Task`/`PowerOffVM_Task` 병렬 호출 | — | 중 | 동시 요청 처리·Task 병렬성 검증 대상 | FR-VC-0405, 0406, 0501 |
| CMP-VS-12 | SOAP `PowerOffVM_Task`(또는 `ShutdownGuest`) → `ReconfigVM_Task`(numCPUs, numCoresPerSocket, memoryMB) → `PowerOnVM_Task` / REST `PATCH /api/vcenter/vm/{vm}/hardware/cpu`, `PATCH …/hardware/memory` | — | 상 | '순차 재기동' = Hot-add 미사용 시 전원 끔 → 변경 → 켬 순서 | FR-VC-0404, 0417 |
| CMP-VS-13 | SOAP `GuestOperationsManager.processManager.StartProgramInGuest`(게스트 내부 IP 변경) 또는 `CustomizeVM_Task` / 서브넷 변경 시 `ReconfigVM_Task`(vNIC backing 변경) | Policy Segment Port의 `address_bindings` 갱신(사용 시) | 하 | Running 상태에서만 가능 → Guest Operations 가능성. VMware Tools·perl 요구는 게스트 커스터마이징의 요구사항과 같음 | FR-VC-0416 / FR-NSX-0604 |
| CMP-VS-14 | — | Policy `PATCH /infra/tier-1s/{t1}/nat/USER/nat-rules/{rule}`(action `DNAT`+`SNAT` 쌍 또는 `REFLEXIVE`, `translated_network`=NAT IP, `logging`), `DELETE` 동일 경로 | 중 | NAT 로그의 `RDR`(DNAT)·`NAT OUT`(SNAT) 형식이 근거. Public IP 풀은 CMP-PIP 참조 | 신규 FR 필요 (NAT) |
| CMP-VS-15 | SOAP `ReconfigVM_Task`(vNIC add) / REST `POST /api/vcenter/vm/{vm}/hardware/ethernet` | Policy 로컬 서브넷 Segment(`connectivity_path` 없음) | 중 | — | FR-VC-0404 / FR-NSX-0211 |
| CMP-VS-16 | — | Policy `PATCH /infra/domains/default/groups/{sg}`(PathExpression = Segment Port/VIF, 또는 VM Tag Condition) / VM Tag 방식: Policy `POST /infra/realized-state/enforcement-points/default/virtual-machines?action=update_tags` 또는 MP `POST /fabric/virtual-machines?action=update_tags` | 중 | SG 최대 5개·반영 약 5분. VM 단위인지 vNIC(포트) 단위인지 구현 경로 선정 필요(실제 CMP 경로 미확인) | FR-NSX-0215, 0219, 0226, 0229 |
| CMP-VS-17 | SOAP `ReconfigVM_Task`(VirtualDisk `operation=add, fileOperation=create` / `operation=edit`로 capacityInKB 증가) 또는 `VirtualDiskManager.ExtendVirtualDisk_Task` / REST `POST /api/vcenter/vm/{vm}/hardware/disk` | — | 상 | 축소 불가는 vSphere 디스크 동작과 같음 | FR-VC-0404 |
| CMP-VS-18 | SOAP `ReconfigureComputeResource_Task`(ClusterAntiAffinityRuleSpec 추가·수정·삭제) | — | 중 | — | (DRS 규칙 FR 없음) |
| CMP-VS-19 | SOAP `PowerOffVM_Task` → `Destroy_Task` / REST `DELETE /api/vcenter/vm/{vm}` | Policy NAT 규칙·Group 멤버십·VM Tag 정리, Segment Port 자동 삭제 확인 | 상 | 삭제 후 조회 시 Not Found 계약 | FR-VC-0409, 0417 / FR-NSX-0415, 0608 |
| CMP-VS-20 | SOAP `CloneVM_Task`(spec.template=true) 또는 `MarkAsTemplate` / REST `POST /api/vcenter/vm-template/library-items`(VM → 템플릿 항목), `POST /api/vcenter/ovf/library-item`(VM → OVF 항목) | — | 중 | VM 중지 요구 → OVF 내보내기 또는 템플릿 복제 방식 추정 | FR-VC-0415, 0206 |
| CMP-VS-21 | REST `GET /api/content/library/item/{item}`, `GET /api/vcenter/vm-template/library-items/{item}` / SOAP 템플릿 속성 조회 | — | 중 | — | FR-VC-0206 |
| CMP-VS-22 | REST `POST /api/content/library/item/{item}?action=copy` (또는 CMP 내부 권한 공유) | — | 하 | 승인 절차는 CMP 기능 | FR-VC-0206 |
| CMP-VS-23 | REST `DELETE /api/content/library/item/{item}` / SOAP `Destroy_Task`(템플릿) | — | 중 | — | FR-VC-0206, 0409 |
| CMP-VS-24 | REST `POST /api/content/library/item/update-session` → `POST /api/content/library/item/update-session/{updateSessionId}/file`(source_type `PULL`, URL) → `POST /api/content/library/item/update-session/{session}?action=complete` → `POST /api/vcenter/ovf/library-item/{item}?action=filter` / 또는 SOAP `OvfManager.CreateImportSpec` + `ResourcePool.ImportVApp` + `HttpNfcLease`(`HttpNfcLeaseProgress`, `HttpNfcLeaseComplete`) / 변환: `ReconfigVM_Task`(기존 vNIC 제거, vmxnet3 추가) | — | 중 | HW 19 이하·SCSI 0:0 등 CMP 상품 제약은 제외. 채택한 OVF 경로는 공식 버전 계약만 검증 | FR-VC-0206, 0211 |
| CMP-VS-25 | REST `GET`/`DELETE /api/content/library/item/{item}` | — | 중 | — | FR-VC-0206 |
| CMP-VS-26 | SOAP `ReconfigureComputeResource_Task`(ClusterVmGroup·ClusterHostGroup·ClusterVmHostRuleInfo), 디스크 분산: 데이터스토어 선택 또는 Storage DRS `VirtualDiskAntiAffinityRuleSpec` | — | 하 | 'Rack·Block Storage 분산 최대 3' 근거. 구현 방식 다양 | (DRS 규칙 FR 없음) |
| CMP-VS-27 | CMP 스케줄러 + CMP-VS-13과 같은 API(VM별 반복) | CMP-VS-13과 같음 | 하 | — | FR-VC-0416 |
| CMP-VS-28 | (선택) SOAP `ReconfigVM_Task`(`extraConfig` `guestinfo.*`) | — | 하 | CMP 별도 메타데이터 서비스(IMDS)로 판단 | — |
| CMP-VS-29 | SOAP `GuestOperationsManager.processManager.StartProgramInGuest` (또는 AWX가 SSH로 직접 실행) | — | 하 | AWX 경유로 명시 → vCenter API를 쓰지 않을 수 있음 | — |

### 17.4 Block Storage (VM)

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-BS-01 | SOAP `ReconfigVM_Task`(VirtualDisk add, `fileOperation=create`, `capacityInKB`, datastore, `controllerKey`·`unitNumber`, `profile`=PBM 정책; Multi Attach는 `backing.sharing=sharingMultiWriter`+Eager Zeroed Thick) / REST `POST /api/vcenter/vm/{vm}/hardware/disk` / PBM `PbmQueryProfile` | — | 상 | 서버당 15개 = SCSI 컨트롤러 1개의 장치 슬롯 수(16 중 7번 예약)와 일치 | FR-VC-0404, 0305, 0210 |
| CMP-BS-02 | SOAP `RetrievePropertiesEx`(VM `config.hardware.device`의 VirtualDisk, `layoutEx.disk`) / REST `GET /api/vcenter/vm/{vm}/hardware/disk` | — | 중 | Resizing·Editing 상태 = 진행 중 Reconfig Task | FR-VC-0305 |
| CMP-BS-03 | SOAP `ReconfigVM_Task`(VirtualDisk `operation=edit`, capacityInKB 증가) 또는 `ExtendVirtualDisk_Task` | — | 상 | 축소 불가 | FR-VC-0404 |
| CMP-BS-04 | SOAP `ReconfigVM_Task`(다른 VM에 기존 VMDK `fileName` 지정 add / `operation=remove`, fileOperation 없음) | — | 상 | Multi-writer 공유 디스크 연결·해제 | FR-VC-0404 |
| CMP-BS-05 | SOAP `ReconfigVM_Task`(`operation=remove`, `fileOperation=destroy`) 또는 `VirtualDiskManager.DeleteVirtualDisk_Task` / REST `DELETE /api/vcenter/vm/{vm}/hardware/disk/{disk}` | — | 상 | 연결 서버 해제 선행 | FR-VC-0404 |

### 17.5 GPU Server

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-GPU-01 | CMP-VS-02 + SOAP `ReconfigVM_Task`(`VirtualPCIPassthrough`, backing `VirtualPCIPassthroughVmiopBackingInfo.vgpu` 또는 `VirtualPCIPassthroughDynamicBackingInfo`, `memoryReservationLockedToMax=true`) / 호스트 GPU 조회 `HostSystem.config.graphicsInfo`, `QueryConfigTarget` | CMP-VS-03과 같음 | 중 | Passthrough·vGPU 중 무엇인지 확인 필요 | (GPU FR 없음) |
| CMP-GPU-02 | CMP-VS-07~17과 같음 | CMP-VS-14·16과 같음 | 중 | — | VS와 같음 |
| CMP-GPU-03 | CMP-VS-20~23과 같음 | — | 중 | — | VS와 같음 |
| CMP-GPU-04 | SOAP `ReconfigVM_Task`(vGPU MIG 프로파일, 예: `grid_a100-*c`) | — | 하 | 게스트 내부 MIG 설정은 API 대상 아님 | (GPU FR 없음) |
| CMP-GPU-05 | SOAP `ReconfigVM_Task`(다중 GPU Passthrough 장치) | — | 하 | Fabric Manager 활성화는 게스트 작업 | (GPU FR 없음) |

### 17.6 Auto-Scaling

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-AS-01 | 없음 (CMP 템플릿) | — | 중 | VM 생성 파라미터 저장 | — |
| CMP-AS-02 | Desired 수만큼 CMP-VS-02~04 반복 | CMP-VS-14·16, CMP-AS-05 | 중 | — | FR-VC-0401 |
| CMP-AS-03 | SOAP `PerformanceManager.QueryPerf`(cpu.usage.average, mem.usage.average 등), `QueryAvailablePerfMetric` 또는 `summary.quickStats` 조회 / 스케일 인: CMP-VS-19 | — | 하 | 지표 수집을 CMP 모니터링이 별도로 수행할 수 있음 | FR-MET |
| CMP-AS-04 | CMP 스케줄러 + CMP-VS-02·19 | — | 중 | — | — |
| CMP-AS-05 | — | Policy `PATCH /infra/lb-pools/{pool}`(`members` 갱신 또는 `member_group`=Group 경로), `PATCH /infra/domains/default/groups/{grp}` | 중 | `ASG_xxxxxx` 자동 서버그룹 = LB Pool | 신규 FR 필요 (LB) |
| CMP-AS-06 | 없음 | 없음 | 상 | CMP 내부 | — |
| CMP-AS-07 | CMP-VS-19 반복 | CMP-AS-05 정리 | 중 | — | FR-VC-0409 |

### 17.7 VPC · 서브넷 · Public IP · IGW · NAT

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-VPC-01 | — | Policy `PATCH /infra/tier-1s/{t1}`, `PATCH /infra/tier-1s/{t1}/locale-services/{ls}`(`edge_cluster_path`), DNS Service IP: `PATCH /infra/tier-1s/{t1}/dns-forwarder`(`listener_ip`) | 중 | DNS 포워더 listener IP가 'DNS Service IP 자동 설정'과 대응. NSX VPC 경로는 대상 버전(NSX-T 3.x)에 없음 | FR-NSX-0213 |
| CMP-VPC-02 | — | Policy `GET /infra/tier-1s/{t1}`, `GET /infra/segments?…`, `GET /infra/tier-1s/{t1}/static-routes`, Realization 상태 | 중 | — | FR-NSX-0213, 0401, 0402 |
| CMP-VPC-03 | — | Policy `DELETE /infra/tier-1s/{t1}/locale-services/{ls}`, `DELETE /infra/tier-1s/{t1}` (하위 자원 존재 시 의존성 오류) | 중 | 삭제 의존관계 검사 | FR-NSX-0415 |
| CMP-VPC-04 | — | Rule·NAT의 `logged`/`logging` 필드, LB `access_log_enabled` / (선택) MP `POST /node/services/syslog/exporters`, `…/transport-nodes/{edge}/node/services/syslog/exporters` | 하 | 로그를 Object Storage로 모으는 경로는 SCP 내부 구성 | 신규 FR 필요 (로깅) |
| CMP-VPC-05 | — | Policy `PATCH /infra/dns-forwarder-zones/{zone}`(`dns_domain_names`, `upstream_servers`, `source_ip`), `PATCH /infra/tier-1s/{t1}/dns-forwarder`(`conditional_forwarder_zone_paths`), `DELETE /infra/dns-forwarder-zones/{zone}` | 상 | 도메인·Source IP·DNS 서버 IP 입력과 최대 3개 제한이 NSX 조건부 포워더 존(upstream 최대 3개, `source_ip`)과 일치 | 신규 FR 필요 (DNS Forwarder) |
| CMP-SUB-01 | — | Policy `PATCH /infra/segments/{seg}`(`connectivity_path`={t1}, `subnets[].gateway_address`, `transport_zone_path`) — 로컬 서브넷은 `connectivity_path` 없음. Public/Private 구분은 Tier-1 `route_advertisement_types`/`route_advertisement_rules` 또는 NAT 허용 여부 | 중 | Gateway 자동 지정 = 서브넷 첫 주소를 `gateway_address`로 설정 | FR-NSX-0211, 0403, 0410 |
| CMP-SUB-02 | (선택) SOAP Network·DVPortgroup 조회 | Policy `GET /infra/segments/{seg}`, `GET /infra/segments/{seg}/ports`, Realization 상태 | 중 | 사용 중 IP = Segment Port 주소 바인딩 또는 CMP IPAM | FR-NSX-0211, 0603 |
| CMP-SUB-03 | — | CMP-NAT-01과 같음 | 중 | — | 신규 FR 필요 (NAT) |
| CMP-SUB-04 | — | VIP 예약: CMP IPAM 또는 Policy `PATCH /infra/segments/{seg}/ports/{port}`(`address_bindings`) / VIP NAT: CMP-VS-14 / VIP SG: Group의 `IPAddressExpression` | 하 | — | FR-NSX-0215 |
| CMP-SUB-05 | — | Policy `DELETE /infra/segments/{seg}` (포트·참조 존재 시 거부) | 상 | — | FR-NSX-0406, 0415 |
| CMP-PIP-01 | — | Policy `PATCH /infra/ip-pools/{pool}/ip-allocations/{alloc}` 또는 CMP IPAM | 하 | 공용·Secured IGW가 같은 Pool 사용 | 신규 FR 필요 (IP Pool) |
| CMP-PIP-02 | — | Policy `GET /infra/ip-pools/{pool}/ip-allocations` | 하 | Attached = NAT 규칙에서 사용 중 | 신규 FR 필요 (IP Pool) |
| CMP-PIP-03 | — | Policy `DELETE /infra/ip-pools/{pool}/ip-allocations/{alloc}` | 하 | — | 신규 FR 필요 (IP Pool) |
| CMP-IGW-01 | — | Policy `PATCH /infra/tier-1s/{t1}`(`tier0_path`={공용/전용/Secure T0 또는 VRF}, `route_advertisement_types`), Firewall 사용 시 CMP-FW-01 | 중 | 공용·전용·Secure IGW = 서로 다른 Tier-0/VRF로 추정 | FR-NSX-0212, 0213 |
| CMP-IGW-02 | — | Tier-1→Tier-0 연결 시 기본 경로 자동 / 또는 Policy `PATCH /infra/tier-1s/{t1}/static-routes/{route}`(`network`=`0.0.0.0/0`) | 하 | 기본 경로를 시스템 규칙으로 고정 | FR-NSX-0213 |
| CMP-IGW-03 | — | Policy `GET /infra/tier-1s/{t1}`, Realization 상태 | 중 | Attaching/Detaching = Realization 진행 | FR-NSX-0220 |
| CMP-IGW-04 | — | Policy `PATCH /infra/tier-1s/{t1}`(`tier0_path` 설정/제거) | 중 | — | FR-NSX-0213 |
| CMP-IGW-05 | — | CMP-IGW-04 + Gateway Firewall 정책 삭제 | 중 | 의존 자원 존재 시 거부 | FR-NSX-0415 |
| CMP-NAT-01 | — | Policy `PATCH /infra/tier-1s/{t1}/nat/USER/nat-rules/{rule}`(action `SNAT`, `source_network`=서브넷 CIDR, `translated_network`=대표 공인 IP) | 상 | '서브넷 단위 대표 공인 IP 1개, 외부→내부 접속 불가' = SNAT 규칙 정의와 같음 | 신규 FR 필요 (NAT) |
| CMP-NAT-02 | — | Policy `GET /infra/tier-1s/{t1}/nat/USER/nat-rules/{rule}` | 상 | — | 신규 FR 필요 (NAT) |
| CMP-NAT-03 | — | Policy `DELETE /infra/tier-1s/{t1}/nat/USER/nat-rules/{rule}` | 상 | — | 신규 FR 필요 (NAT) |

### 17.8 VPC Peering · Routing Table · Endpoint · Dedicated Edge Host

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-PEER-01 | — | Policy 연결용 Segment `PATCH /infra/segments/{transit}` + 양쪽 Tier-1 Service Interface `PATCH /infra/tier-1s/{t1}/locale-services/{ls}/interfaces/{if}` (또는 Tier-0/VRF 경유), Firewall 사용 시 CMP-FW-01 | 하 | 요청·승인 절차는 CMP 기능. 데이터 경로 추정은 미확인이며 자체 API 시뮬레이션 경로만 선정 | FR-NSX-0211, 0214 |
| CMP-PEER-02 | — | CMP-PEER-01 실행(승인 시점) | 하 | — | — |
| CMP-PEER-03 | — | 관련 Interface·Static Route 조회 | 하 | — | FR-NSX-0214 |
| CMP-PEER-04 | — | Interface·Segment·Static Route 삭제 | 하 | 방화벽·라우팅 규칙 선삭제는 의존성 검사 | FR-NSX-0415 |
| CMP-RT-01 | — | Policy `GET /infra/tier-1s/{t1}/static-routes` | 중 | VPC 생성 시 자동 생성 = Tier-1 기본 라우팅 | FR-NSX-0213 |
| CMP-RT-02 | — | Policy `PATCH /infra/tier-1s/{t1}/static-routes/{route}`(`network`, `next_hops[].ip_address`=TGW/DC/Peering 연결 인터페이스) | 중 | 중복·하위 대역 검사는 CMP 또는 NSX 검증 | 신규 FR 필요 (Static Route) |
| CMP-RT-03 | — | Policy `DELETE /infra/tier-1s/{t1}/static-routes/{route}` | 중 | — | 신규 FR 필요 (Static Route) |
| CMP-EP-01 | — | Policy Tier-1 NAT(`DNAT`, Endpoint IP → 서비스 IP) 또는 Service Interface | 하 | SCP 서비스망 연결 구현은 내부 구성 | 신규 FR 필요 (NAT) |
| CMP-EP-02 | — | CMP-EP-01 규칙 조회·삭제 | 하 | — | — |
| CMP-EDGE-01 | — | MP `POST /transport-nodes`(Edge 노드 배포), `POST /edge-clusters` / Policy `GET /infra/sites/default/enforcement-points/default/edge-clusters` | 하 | '장비 구축 완료 후 신청' → 실제 Edge 배포는 운영자 작업, CMP는 Edge Cluster 매핑만 할 가능성 | FR-NSX-0206 |
| CMP-EDGE-02 | — | MP `GET /edge-clusters/{id}`, `GET /transport-nodes/{id}/state` | 하 | — | FR-NSX-0206 |

### 17.9 Security Group

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-SG-01 | — | Policy `PATCH /infra/domains/default/groups/{sg}`(멤버 정의) + `PATCH /infra/domains/default/security-policies/{sg}`(`category`=Application, `scope`=[Group]) | 상 | 'VPC 내부 수신·발신 트래픽을 서버 단위로 제어'하는 가상 방화벽 = DFW | FR-NSX-0215, 0217 |
| CMP-SG-02 | — | Policy `GET /infra/domains/default/security-policies/{sg}`, `GET …/rules`, `GET /infra/domains/default/groups/{sg}/members/virtual-machines`(또는 `…/members/ip-addresses`) | 상 | — | FR-NSX-0217, 0218 |
| CMP-SG-03 | — | Policy 정책 마지막 규칙(`action`=`DROP`, source·destination ANY, `scope`=SG Group) | 상 | 로그의 'default any-any deny rule은 시스템 생성 ID' 설명과 일치 | FR-NSX-0218 |
| CMP-SG-04 | — | Policy `PATCH /infra/domains/default/security-policies/{sg}/rules/{rule}`(`direction` IN/OUT, `source_groups`/`destination_groups`에 IP·CIDR 직접 지정, `service_entries`(L4PortSetServiceEntry·ICMPTypeServiceEntry) 또는 `services`, `action`=ALLOW, `logged`) | 상 | 프로토콜 ALL = `ANY` 서비스 | FR-NSX-0216, 0218, 0405 |
| CMP-SG-05 | — | Policy `PATCH /infra/domains/default/security-policies/{sg}`(`rules` 배열) 또는 Hierarchical API `PATCH /infra` | 중 | 100개 일괄 등록 | FR-NSX-0223, 0405 |
| CMP-SG-06 | — | Policy `GET/PATCH …/security-policies/{sg}/rules/{rule}`, `_revision` 충돌 처리 | 상 | — | FR-NSX-0218, 0409 |
| CMP-SG-07 | — | Policy `DELETE …/security-policies/{sg}/rules/{rule}` | 상 | Error 상태 = Realization 실패 | FR-NSX-0218, 0504 |
| CMP-SG-08 | — | Policy Group `IPAddressExpression`(멀티캐스트 주소) 추가·삭제 | 중 | — | FR-NSX-0215 |
| CMP-SG-09 | — | Policy 규칙 `logged` 일괄 변경 | 중 | — | FR-NSX-0218 |
| CMP-SG-10 | — | Policy `DELETE /infra/domains/default/security-policies/{sg}`, `DELETE /infra/domains/default/groups/{sg}` | 상 | 적용 자원 존재 시 거부 | FR-NSX-0415 |
| CMP-SG-11 | — | (데이터플레인) DFW 패킷 로그 — API 대상 아님 | 상 | `INET match PASS/DROP`, `TERM` 형식 = NSX DFW `dfwpktlogs` 형식 | — |

### 17.10 Firewall

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-FW-01 | — | Policy `PATCH /infra/domains/default/gateway-policies/{fw}`(`scope`=[{t1} 또는 연결 인터페이스 경로]) | 중 | 연계 상품별 1개 Firewall = 게이트웨이(또는 인터페이스) 범위의 Gateway Policy | 신규 FR 필요 (Gateway Firewall) |
| CMP-FW-02 | — | Policy `GET /infra/domains/default/gateway-policies/{fw}`, `GET …/rules` | 중 | — | 신규 FR 필요 |
| CMP-FW-03 | — | Policy `PATCH /infra/tier-1s/{t1}`(`disable_firewall`, 지원 버전 확인) 또는 Gateway Policy 규칙 일괄 활성/비활성 | 하 | Active/InActive 대응 방식 확인 필요 | 신규 FR 필요 |
| CMP-FW-04 | — | Policy `PATCH /infra/domains/default/gateway-policies/{fw}/rules/{rule}`(`source_groups`, `destination_groups`, `services`/`service_entries`, `action` ALLOW/DROP, `direction` IN/OUT/IN_OUT, `sequence_number`, `logged`) + 위치 지정: `POST …/rules/{rule}?action=revise&operation=insert_before\|insert_after\|insert_bottom&anchor_path=…` | 상 | 동작 Allow/Deny, 방향 In-Out, 기준 규칙 앞·뒤 삽입이 NSX 규칙 모델·revise 연산과 일치 | 신규 FR 필요 |
| CMP-FW-05 | — | Policy `PATCH /infra/domains/default/gateway-policies/{fw}`(`rules` 배열, `disabled=true`) | 중 | 일괄 등록 시 비활성 = `disabled` 필드 | 신규 FR 필요 |
| CMP-FW-06 | — | Policy `PATCH …/rules/{rule}`(`disabled`, `description` 등) | 상 | — | 신규 FR 필요 |
| CMP-FW-07 | — | Policy `POST …/rules/{rule}?action=revise&operation=insert_before\|insert_after&anchor_path=…` | 상 | — | 신규 FR 필요 |
| CMP-FW-08 | — | Policy `DELETE …/rules/{rule}` (다건은 반복 또는 정책 PATCH) | 상 | — | 신규 FR 필요 |
| CMP-FW-09 | — | Policy 규칙 `logged` 변경 | 중 | — | 신규 FR 필요 |
| CMP-FW-10 | — | (데이터플레인) Gateway Firewall 로그 — API 대상 아님 | 상 | `reason-match PASS/DROP/RDR` 형식 | — |

### 17.11 Load Balancer

대상이 NSX-T 3.x이므로 NSX 네이티브 LB(`/infra/lb-*`) API를 계약 기준으로 사용한다. NSX 3.2에서 네이티브 LB의 향후 지원 종료(deprecation)가 공지되고 NSX Advanced Load Balancer(Avi)로의 전환이 권고되었지만, 3.x 범위에서는 API가 제공된다. 3.0 / 3.1 / 3.2 사이의 필드 차이는 버전 프로파일로 처리하고, 대상 범위를 4.x로 넓힐 때 FR-NSX-0231에 따라 재판정한다.

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-LB-01 | — | Policy `PATCH /infra/lb-services/{lb}`(`connectivity_path`={t1}, `size` SMALL/MEDIUM/LARGE), LB Link IP: Pool `snat_translation`(LBSnatIpPool) | 상 | 크기별 서비스 수 20/100/1,000 = NSX LB Service 크기별 Virtual Server 한도와 일치. LB Link IP = 서버 측 SNAT IP | 신규 FR 필요 (LB) |
| CMP-LB-02 | — | Policy `GET /infra/lb-services/{lb}`, `GET /infra/lb-services/{lb}/detailed-status`, `GET /infra/lb-virtual-servers?…` | 상 | — | 신규 FR 필요 |
| CMP-LB-03 | — | Policy `DELETE /infra/lb-services/{lb}` | 상 | 하위 자원 존재 시 거부 | 신규 FR 필요 |
| CMP-LBS-01 | — | Policy `PATCH /infra/lb-virtual-servers/{vs}`(`lb_service_path`, `ip_address`, `ports`, `default_pool_member_ports`, `pool_path`, `application_profile_path`) + NAT IP는 CMP-VS-14와 같은 Tier-1 NAT | 상 | L4 TCP/UDP·L7 HTTP/HTTPS = LBFastTcp/LBFastUdp/LBHttp 프로파일 | 신규 FR 필요 |
| CMP-LBS-02 | — | Policy Virtual Server `rules[]`(LBRule: `match_conditions`=LBHttpRequestUriCondition `match_type`=STARTS_WITH, `actions`=LBSelectPoolAction), 기본 패턴 = `pool_path` | 상 | '앞부분 일치·위에서부터 첫 일치' = LBRule 평가 순서 | 신규 FR 필요 |
| CMP-LBS-03 | — | Policy Virtual Server `lb_persistence_profile_path`, `application_profile_path`(LBHttpProfile `http_redirect_to`/`http_redirect_to_https`) | 상 | — | 신규 FR 필요 |
| CMP-LBS-04 | (선택) REST 인증서 조회 없음 | Policy `PATCH /infra/certificates/{cert}`(Certificate Manager 동기화), `PATCH /infra/lb-client-ssl-profiles/{p}`·`/infra/lb-server-ssl-profiles/{p}`(`ciphers`, `protocols`=TLS_V1_2), Virtual Server `client_ssl_profile_binding`·`server_ssl_profile_binding` | 상 | 보안 수준별 Cipher 목록이 NSX SSL 프로파일 Cipher 열거값과 일치 | 신규 FR 필요 |
| CMP-LBS-05 | — | Policy Virtual Server `access_log_enabled` | 중 | — | 신규 FR 필요 |
| CMP-LBS-06 | — | Policy `GET /infra/lb-services/{lb}/lb-virtual-servers/{vs}/detailed-status`, `PATCH /infra/lb-virtual-servers/{vs}` | 상 | 상태 UP/PARTIALLY_UP/DOWN/DETACHED/UNKNOWN = NSX Virtual Server 상태 열거값 | 신규 FR 필요 |
| CMP-LBS-07 | — | Policy `DELETE /infra/lb-virtual-servers/{vs}` | 상 | — | 신규 FR 필요 |
| CMP-LBG-01 | — | Policy `PATCH /infra/lb-pools/{pool}`(`algorithm` ROUND_ROBIN/WEIGHTED_ROUND_ROBIN/LEAST_CONNECTION/WEIGHTED_LEAST_CONNECTION/IP_HASH, `members[]` `ip_address`·`port`·`weight`·`admin_state` ENABLED/DISABLED/GRACEFUL_DISABLED) | 상 | 알고리즘 5종·가중치 1~256·멤버 상태 3종이 NSX LBPool 모델과 일치 | 신규 FR 필요 |
| CMP-LBG-02 | — | Policy `PATCH /infra/lb-monitor-profiles/{mon}`(LBHttpMonitorProfile/LBTcpMonitorProfile/LBUdpMonitorProfile/LBIcmpMonitorProfile: `monitor_port`, `interval`, `timeout`, `fall_count`·`rise_count`, `request_method`, `request_url`, `request_body`, `response_body`, `request_version`), Pool `active_monitor_paths` | 상 | HTTP 1.1 사용 여부 = `request_version` | 신규 FR 필요 |
| CMP-LBG-03 | — | Policy `GET /infra/lb-pools/{pool}`, `GET /infra/lb-services/{lb}/lb-pools/{pool}/detailed-status`, `PATCH /infra/lb-pools/{pool}` | 상 | — | 신규 FR 필요 |
| CMP-LBG-04 | — | Policy `DELETE /infra/lb-pools/{pool}` | 상 | Virtual Server 참조 시 거부 | 신규 FR 필요 |
| CMP-LBP-01 | — | Policy `PATCH /infra/lb-persistence-profiles/{p}`(LBSourceIpPersistenceProfile / LBCookiePersistenceProfile `cookie_name`) | 상 | — | 신규 FR 필요 |
| CMP-LBP-02 | — | Policy `PATCH /infra/lb-app-profiles/{p}`(LBFastTcpProfile / LBFastUdpProfile / LBHttpProfile `request_header_size`·`response_header_size` 등) | 상 | 헤더 크기 기본값 1024/4096 byte = LBHttpProfile 기본값 | 신규 FR 필요 |
| CMP-LBP-03 | — | Policy `GET`/`PATCH`/`DELETE` 위 경로 | 상 | — | 신규 FR 필요 |

### 17.12 Transit Gateway · Direct Connect · VPN

| ID | vCenter API | NSX API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-TGW-01 | — | Policy `PATCH /infra/tier-0s/{tgw}`(VRF: `vrf_config.tier0_path`) 또는 전용 Tier-0, `PATCH /infra/tier-0s/{tgw}/locale-services/{ls}` | 하 | TGW = VRF Gateway로 추정 | FR-NSX-0212 |
| CMP-TGW-02 | — | Policy Transit Segment + `PATCH /infra/tier-1s/{t1}/locale-services/{ls}/interfaces/{if}` 또는 `…/tier-0s/{tgw}/locale-services/{ls}/interfaces/{if}`, Firewall 사용 시 CMP-FW-01 | 하 | VPC가 IGW와 TGW 3개에 동시에 연결 가능 → Tier-1 `tier0_path` 하나로는 표현 불가, 인터페이스 방식 추정 | FR-NSX-0214 |
| CMP-TGW-03 | — | (회선 개통 후) Policy `PATCH /infra/tier-0s/{tgw}/locale-services/{ls}/interfaces/{uplink}`(EXTERNAL, VLAN Segment), `PATCH …/bgp`, `PATCH …/bgp/neighbors/{nbr}` | 하 | 물리 회선 작업은 범위 외. BGP 설정 API만 해당 | 신규 FR 필요 (BGP) |
| CMP-TGW-04 | — | Policy `PATCH`/`DELETE /infra/tier-0s/{tgw}/static-routes/{route}` | 중 | — | 신규 FR 필요 (Static Route) |
| CMP-TGW-05 | — | Policy 두 TGW(VRF) 간 Transit Segment·Interface·Static Route | 하 | — | FR-NSX-0214 |
| CMP-TGW-06 | — | Policy `GET`/`DELETE` 위 리소스, Realization 상태 | 하 | — | FR-NSX-0415 |
| CMP-DC-01 | — | Policy `PATCH /infra/tier-0s/{dc}`(VRF) | 하 | TGW와 같은 구조로 추정 | FR-NSX-0212 |
| CMP-DC-02 | — | CMP-TGW-02와 같음 | 하 | Direct Connect당 VPC 1개 | FR-NSX-0214 |
| CMP-DC-03 | — | CMP-TGW-03과 같음 | 하 | — | 신규 FR 필요 (BGP) |
| CMP-DC-04 | — | Policy `PATCH`/`DELETE /infra/tier-0s/{dc}/static-routes/{route}` | 중 | — | 신규 FR 필요 (Static Route) |
| CMP-DC-05 | — | Policy `GET`/`DELETE` 위 리소스 | 하 | — | FR-NSX-0415 |
| CMP-VPN-01 | — | Policy `PATCH /infra/tier-1s/{t1}/locale-services/{ls}/ipsec-vpn-services/{svc}`, `…/local-endpoints/{le}`(공인 IP), Local Subnet = `PATCH /infra/segments/{seg}`, QoS = `PATCH /infra/gateway-qos-profiles/{qos}`(`committed_bandwidth`) + Tier-1 `qos_profile` | 중 | 프로젝트당 1개 GW = 전용 Tier-1(VPN GW)로 추정 | 신규 FR 필요 (IPsec VPN, QoS) |
| CMP-VPN-02 | — | Policy `GET` 위 리소스, `PATCH /infra/gateway-qos-profiles/{qos}` | 중 | — | 신규 FR 필요 |
| CMP-VPN-03 | — | Policy `PATCH /infra/tier-1s/{t1}/locale-services/{ls}/ipsec-vpn-services/{svc}/sessions/{session}`(PolicyBasedIPSecVpnSession 또는 RouteBasedIPSecVpnSession: `peer_address`, `peer_id`, `psk`, `rules`), 프로파일 `PATCH /infra/ipsec-vpn-ike-profiles/{p}`, `/infra/ipsec-vpn-tunnel-profiles/{p}`, `/infra/ipsec-vpn-dpd-profiles/{p}`, 상태 `GET …/sessions/{session}/detailed-status` | 중 | 'IPsec 암호화 가상 터널' 명시 | 신규 FR 필요 |
| CMP-VPN-04 | SOAP `ReconfigVM_Task`(VPN Local Subnet용 vNIC 추가·제거) | Policy Local Subnet Segment | 중 | — | FR-VC-0404 / FR-NSX-0211 |
| CMP-VPN-05 | — | Policy `DELETE` 세션·Local Endpoint·서비스·Segment 순서 | 중 | — | 신규 FR 필요 |

### 17.13 Virtual Server DR · Backup (범위 외 후보)

vCenter·NSX 코어 API가 아니라 vSphere Replication(VR)·Site Recovery Manager(SRM)·백업 솔루션(VADP 사용) API에 해당한다. 경로는 제품 버전별 문서로 확인이 필요하다.

| ID | vCenter API | 기타 VMware API | 신뢰도 | 근거·비고 | 관련 FR |
|---|---|---|---|---|---|
| CMP-DR-01 | — | SRM Protection Group (예: SRM REST `/api/rest/srm/v2/pairings/{pairing}/protection-management/groups`) | 하 | DR Group = Protection Group 추정 | 없음 |
| CMP-DR-02 | — | SRM 인벤토리 매핑(네트워크 매핑) | 하 | Network Pairing = 네트워크 매핑 추정 | 없음 |
| CMP-DR-03 | — | vSphere Replication 복제 구성 (예: VR REST `/api/rest/vr/v2/pairings/{pairing}/replications`, `rpo`, MPIT 인스턴스 수) | 중 | 동기화 주기 5분~24시간 = VR RPO 범위, 복구 시점 최대 24개 = VR MPIT 최대 인스턴스 수와 일치 | 없음 |
| CMP-DR-04 | — | VR 복제 조회·삭제 | 중 | — | 없음 |
| CMP-DR-05 | — | SRM Recovery Plan 생성·조회·삭제 | 하 | — | 없음 |
| CMP-DR-06 | — | SRM Recovery Plan 테스트·정리(test / cleanup) | 중 | 모의 훈련 시작·자원 정리 = SRM Test·Cleanup | 없음 |
| CMP-DR-07 | — | SRM Recovery Plan 복구(recovery)·재보호 | 중 | — | 없음 |
| CMP-BK-01 | SOAP `ReconfigVM_Task`(`changeTrackingEnabled=true`), `CreateSnapshot_Task`(quiesce) | 백업 솔루션 스케줄·저장소 API | 하 | Incremental = CBT(`QueryChangedDiskAreas`) 기반 추정, Multi Attach VM Incremental 불가 제약과 일치 | FR-VC-0412 |
| CMP-BK-02 | — | 백업 솔루션 API | 하 | — | 없음 |
| CMP-BK-03 | — | 백업 솔루션 API | 하 | — | 없음 |
| CMP-BK-04 | SOAP `CreateVM_Task` 또는 `ResourcePool.ImportVApp`+`HttpNfcLease`, NFC/VDDK 쓰기, `ReconfigVM_Task` | 백업 솔루션 복구 API | 하 | — | FR-VC-0401, 0211 |
| CMP-BK-05 | SOAP `CancelTask`, `RemoveSnapshot_Task` | 백업 솔루션 API | 하 | — | FR-VC-0414 |

---

## 18. PRD-1 대비 차이 (매핑 결과)

### 18.1 PRD-1에 없는 API 영역

§17 매핑에서 '신규 FR 필요'로 표시한 후보 영역이다. 해당 기능을 시뮬레이션 범위로 채택할 때만 PRD-1 요구사항·작업·자체 테스트를 추가한다. '핵심/확장'은 참고 목록의 분류이며 현재 구현 의무가 아니다.

| 영역 | 관련 CMP 기능 | 주요 Policy 경로 | 영향 범위 |
|---|---|---|---|
| NAT 규칙 | VS-14, SUB-03, NAT-01~03, EP-01, LBS-01 | `/infra/tier-1s/{t1}/nat/USER/nat-rules` | 핵심 |
| Gateway Firewall | FW-01~10, IGW-01, PEER-01, TGW-02, DC-02, LB-01 | `/infra/domains/default/gateway-policies` | 핵심 |
| Load Balancer | LB-01~03, LBS-01~07, LBG-01~04, LBP-01~03, AS-05 | `/infra/lb-services`, `/infra/lb-virtual-servers`, `/infra/lb-pools`, `/infra/lb-monitor-profiles`, `/infra/lb-app-profiles`, `/infra/lb-persistence-profiles`, `/infra/lb-*-ssl-profiles`, `/infra/certificates` | 핵심 (NSX-T 3.x에서 제공, 4.x 확장 시 재판정) |
| DNS Forwarder | VPC-01, VPC-05 | `/infra/tier-1s/{t1}/dns-forwarder`, `/infra/dns-forwarder-zones` | 확장 |
| Static Route | IGW-02, RT-02·03, TGW-04, DC-04 | `/infra/tier-1s/{t1}/static-routes`, `/infra/tier-0s/{t0}/static-routes` | 확장 |
| IP Pool·할당 | PIP-01~03 | `/infra/ip-pools/{pool}/ip-allocations` | 핵심 (CMP IPAM이면 제외) |
| IPsec VPN·Gateway QoS | VPN-01~05 | `/infra/tier-1s/{t1}/locale-services/{ls}/ipsec-vpn-services`, `/infra/ipsec-vpn-*-profiles`, `/infra/gateway-qos-profiles` | 확장 |
| BGP | TGW-03, DC-03 | `/infra/tier-0s/{t0}/locale-services/{ls}/bgp` | 범위 외 후보 |
| 로그 내보내기 | VPC-04 | MP `/node/services/syslog/exporters` | 확장 |

### 18.2 vCenter 측 보완 필요 항목

| 항목 | 관련 CMP 기능 | 내용 | PRD-1 현황 |
|---|---|---|---|
| DRS 규칙 | VS-05, VS-18, VS-26 | `ReconfigureComputeResource_Task`의 Anti-affinity·VM-Host 규칙 | FR 없음 (FR-VC-0302에 DRS '논리적 상태'만 있음) |
| 디스크 공유·암호화 | BS-01, BS-04, VS-02 | Multi-writer 공유, VM Encryption(PBM 정책·KMS) | FR-VC-0210(PBM)만 있음 |
| Guest Operations | VS-13, VS-29 | `GuestOperationsManager` 프로세스 실행 | FR 없음 |
| GPU 장치 | GPU-01, 04, 05 | PCI Passthrough·vGPU 장치 Reconfig | FR 없음 |
| Content Library 업로드 | VS-20, VS-24 | Update Session·URL Pull 업로드, OVF 내보내기·배포 | FR-VC-0206 (MVP 포함 여부 D7 미결) |
| 성능 지표 | AS-03 | `PerformanceManager.QueryPerf` | FR-MET (세부 미정) |

### 18.3 결정 필요 사항

1. **D7 연계:** VM 생성 경로(Clone/Content Library/OVF)와 VS-04 Guest Customization·VS-10 VM 재시작(Reset·Guest Reboot 중 선정)·디스크 변경의 채택 여부를 결정한다. CMP의 실제 경로를 추적하지 않는다. 채택 시 PRD-1의 MVP 표시와 PRD-4 선행 WP·테스트를 함께 갱신한다(참고 문서 1.2 §3.1).
2. **NSX LB 지원 버전:** D1(NSX-T 3.x)로 버전 후보를 정했다. LB 기능의 실제 채택 여부·신규 FR는 별도로 결정한다. 네이티브 LB(`/infra/lb-*`)를 계약 기준으로 사용한다. 첫 기준 조합의 NSX가 NSX-T 3.2.4.3(빌드 24942842)이므로 LB 계약은 이 빌드의 Policy API 스펙을 기준으로 한다. 3.0·3.1과의 필드 차이는 추가 조합에서 다룬다.
3. **DR·Backup:** VR·SRM·백업 솔루션 API는 현재 제외한다. 향후 확대는 별도 범위 변경으로 처리한다.
4. **CMP 내부 기능·상품 제약:** VS-06, VS-28, AS-01·06, COM-05 등의 CMP 내부 처리와 모든 상품 한도·과금·신청/승인·콘솔 상태 재현은 제외한다(D13). SG-11·FW-10의 실제 데이터플레인 로그 생성도 제외한다. VMware API 호출이 있다고 추정되는 나머지 기능도 별도 채택 전에는 의무 범위가 아니다.
