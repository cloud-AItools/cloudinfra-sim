<#
.SYNOPSIS
  Windows 로컬 실행 환경을 구성한다(NFR-ARC-0308, 배포 토폴로지 ① — PRD-4 D6).
.DESCRIPTION
  - hosts 등록: vcenter·esx-01..NN → 127.0.0.2, nsx → 127.0.0.3 (관리자 권한 필요)
  - 루트 CA 신뢰: 기본은 현재 사용자 저장소(관리자 권한 불필요), -MachineStore는 컴퓨터 저장소(관리자 권한 필요)
  Windows 11에서는 127.0.0.0/8 전체가 루프백이라 루프백 별칭 설정이 필요 없고, 443 바인딩에도 관리자 권한이 필요 없다(PoC-19).
.EXAMPLE
  .\Install-CloudinfraSim.ps1 -DataDir "$env:APPDATA\cloudinfra-sim"          # 관리자 PowerShell에서 hosts까지
  .\Install-CloudinfraSim.ps1 -DataDir "$env:APPDATA\cloudinfra-sim" -SkipHosts  # 일반 권한: CA만
#>
param(
  [string]$Domain = 'cloudinfra-sim.test',
  [string]$DataDir = (Join-Path $env:APPDATA 'cloudinfra-sim'),
  [int]$EsxHosts = 3,
  [switch]$MachineStore,
  [switch]$SkipHosts,
  [switch]$Uninstall
)
$ErrorActionPreference = 'Stop'
if ($Domain -notmatch '\.(test|internal)$') { throw "예약 TLD(.test/.internal)만 허용한다(NFR-0314): $Domain" }
$marker = '# cloudinfra-sim'
$hostsPath = Join-Path $env:SystemRoot 'System32\drivers\etc\hosts'
$entries = @("127.0.0.2 vcenter.$Domain $marker", "127.0.0.3 nsx.$Domain $marker")
1..$EsxHosts | ForEach-Object { $entries += ('127.0.0.2 esx-{0:D2}.{1} {2}' -f $_, $Domain, $marker) }

if (-not $SkipHosts) {
  $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
  if (-not $isAdmin) { throw 'hosts 등록에는 관리자 권한이 필요하다. 관리자 PowerShell에서 실행하거나 -SkipHosts를 지정한다.' }
  $current = Get-Content $hostsPath | Where-Object { $_ -notmatch [regex]::Escape($marker) }
  if ($Uninstall) { Set-Content -Path $hostsPath -Value $current -Encoding ascii; Write-Output 'hosts 항목 제거'; }
  else { Set-Content -Path $hostsPath -Value ($current + $entries) -Encoding ascii; Write-Output "hosts 등록: $($entries.Count)건" }
}

$ca = Join-Path $DataDir 'certs\ca.pem'
$store = if ($MachineStore) { 'Cert:\LocalMachine\Root' } else { 'Cert:\CurrentUser\Root' }
if ($Uninstall) {
  Get-ChildItem $store | Where-Object { $_.Subject -like '*cloudinfra-sim Root CA*' } | Remove-Item
  Write-Output "루트 CA 신뢰 제거: $store"
} elseif (Test-Path $ca) {
  Import-Certificate -FilePath $ca -CertStoreLocation $store | Out-Null
  Write-Output "루트 CA 신뢰 등록: $store ($ca)"
} else {
  Write-Output "CA 파일이 아직 없다: $ca — 시뮬레이터를 한 번 기동한 뒤 다시 실행한다."
}
