<#
.SYNOPSIS
  vSphere Automation(REST) 계약 파일 vapi-contract.json을 사용자 측에서 생성한다(SRC-VAPI-SDK-7032, PRD-4 D11 처리).
.DESCRIPTION
  vsphere-automation-sdk-python v7.0.3.2(MIT, archived)의 바인딩·런타임 wheel을 받아 SHA-256을 확인하고,
  .agentic/poc/tools/extract_vapi.py로 계약(오퍼레이션·구조체·열거형·선언 오류·HTTP 매핑)을 추출한다.
  wheel은 설치·실행하지 않고 압축만 풀며, 추출기는 Python ast로 파싱만 한다.
  원본 파일 머리말의 저작권 표기를 확인하기 전까지 결과물은 저장소에 커밋하지 않는다.
.EXAMPLE
  .\Get-VapiContract.ps1 -OutDir D:\AISCP\tools\specs\vapi-bindings-7.0.3.2
  # 시험: $env:VAPI_CONTRACT = 'D:\AISCP\tools\specs\vapi-bindings-7.0.3.2\vapi-contract.json'
#>
param([Parameter(Mandatory = $true)][string]$OutDir)
$ErrorActionPreference = 'Stop'
$base = 'https://raw.githubusercontent.com/vmware-archive/vsphere-automation-sdk-python/v7.0.3.2/lib'
$files = @(
  @{ Url = "$base/vapi-client-bindings/vapi_client_bindings-3.8.0-py2.py3-none-any.whl"; Sha = '9f56d4e0c0a09f372ca1c9cde5a67563aba87d38bbb9f568d42dda9bd2693c29'; Dir = 'bindings' },
  @{ Url = "$base/vapi-runtime/vapi_runtime-2.34.0-py2.py3-none-any.whl"; Sha = '39e02d9c68ccc919a638e38f6e9e78fcf239672fb438c5fcd24e9e6c8b9f89ba'; Dir = 'runtime' }
)
New-Item -ItemType Directory -Force $OutDir | Out-Null
foreach ($f in $files) {
  $whl = Join-Path $OutDir ([IO.Path]::GetFileName($f.Url))
  Invoke-WebRequest -Uri $f.Url -OutFile $whl -UseBasicParsing
  $hash = (Get-FileHash -Algorithm SHA256 $whl).Hash.ToLower()
  if ($hash -ne $f.Sha) { throw "SHA-256 불일치: $whl ($hash)" }
  $dest = Join-Path $OutDir $f.Dir
  # wheel은 zip 형식이지만 Windows PowerShell 5.1의 Expand-Archive는 .whl 확장자를 거부한다.
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
  [IO.Compression.ZipFile]::ExtractToDirectory($whl, $dest)
}
$repo = Resolve-Path (Join-Path $PSScriptRoot '..\..')
$tool = Join-Path $repo '.agentic\poc\tools\extract_vapi.py'
$b = Join-Path $OutDir 'bindings'
$mods = @('com/vmware/cis_client.py', 'com/vmware/vcenter_client.py', 'com/vmware/vcenter/vm_client.py',
  'com/vmware/appliance/system_client.py', 'com/vmware/vapi/std/errors_client.py', 'com/vmware/vapi/std_client.py')
$mods += Get-ChildItem -Recurse (Join-Path $b 'com/vmware/vcenter/vm') -Filter '*_client.py' |
  ForEach-Object { $_.FullName.Substring($b.Length + 1).Replace('\', '/') } | Sort-Object
# Phase 4b: Content Library(FR-VC-0206)
$mods += @('com/vmware/content_client.py', 'com/vmware/content/library_client.py')
$env:VAPI_RUNTIME = Join-Path $OutDir 'runtime'
Push-Location $b
try { & py -3 -I -W ignore $tool . (Join-Path $OutDir 'vapi-contract.json') @mods } finally { Pop-Location }
$out = (Get-FileHash -Algorithm SHA256 (Join-Path $OutDir 'vapi-contract.json')).Hash.ToLower()
Write-Output "vapi-contract.json SHA-256: $out (프로파일 기록값 131da7d5...917ec1과 같으면 동일 추출)"
