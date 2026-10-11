<#
.SYNOPSIS
  NSX-T 3.2.1 공식 OpenAPI 스펙을 사용자 측에서 확보한다(PRD-4 D11: 저장소·배포물에 포함하지 않음).
.DESCRIPTION
  Broadcom 개발자 포털은 서명된 일회성 URL로만 내려받게 하므로, 브라우저로 받은 파일을 검증·배치한다.
  1) 아래 페이지에서 nsx_api.json, nsx_policy_api.json의 Download를 눌러 받는다.
     https://developer.broadcom.com/xapis/nsx-t-data-center-rest-api/3.2.1/x-references/
  2) 이 스크립트에 받은 폴더를 지정한다. SHA-256을 확인한 뒤 NSX 프로파일 폴더로 복사한다.
.EXAMPLE
  .\Get-NsxSpec.ps1 -SourceDir $env:USERPROFILE\Downloads
#>
param(
  [Parameter(Mandatory = $true)][string]$SourceDir,
  [string]$ProfileDir = (Join-Path $PSScriptRoot '..\..\30-implementation\data\compatibility-profiles\nsx\3.2.4.3')
)
$expected = @{
  'nsx_api.json'        = 'acd5b1bac00ac7a555c52967708537a1b8e50a796cfad6fec880b313c5e2fb22'
  'nsx_policy_api.json' = 'f813fa42378b82cd8fa99f250fd02f9415baaf2995ad497cbafe91dc1f8babdb'
}
$failed = $false
foreach ($name in $expected.Keys) {
  $src = Join-Path $SourceDir $name
  if (-not (Test-Path $src)) { Write-Error "$src 가 없다"; $failed = $true; continue }
  $hash = (Get-FileHash -Algorithm SHA256 $src).Hash.ToLower()
  if ($hash -ne $expected[$name]) { Write-Error "$name SHA-256 불일치: $hash"; $failed = $true; continue }
  Copy-Item $src (Join-Path $ProfileDir $name) -Force
  Write-Output "$name 확인·복사 완료"
}
if ($failed) { exit 1 }
