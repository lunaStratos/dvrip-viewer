# 윈도우 exe 에 아이콘을 넣기 위한 리소스(.syso)를 만든다.
# (주의: Windows PowerShell 5.1 은 BOM 없는 UTF-8 을 ANSI 로 읽으므로 출력 문구는 영어로 둔다)
#
#   img/icon.png → winres/icon.ico → rsrc_windows_amd64.syso (저장소 루트)
#
# go build 는 패키지 폴더의 *_windows_amd64.syso 를 자동으로 링크하므로
# 빌드 명령은 그대로 두면 된다. 아이콘을 바꿨을 때만 다시 실행한다.
# 필요: windres (MinGW-w64 / WinLibs gcc 에 포함)
#
# 사용: powershell -ExecutionPolicy Bypass -File winres\make.ps1
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$png  = Join-Path $root 'img\icon.png'
$ico  = Join-Path $PSScriptRoot 'icon.ico'
$rc   = Join-Path $PSScriptRoot 'xmview.rc'
$syso = Join-Path $root 'rsrc_windows_amd64.syso'

Add-Type -AssemblyName System.Drawing

# 1) PNG → 여러 크기를 담은 ICO (각 항목은 PNG 압축, Windows Vista 이상 지원)
$sizes = 16, 24, 32, 48, 64, 128, 256
$src = [System.Drawing.Image]::FromFile($png)
$entries = @()
foreach ($s in $sizes) {
    $bmp = New-Object System.Drawing.Bitmap $s, $s
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.InterpolationMode = 'HighQualityBicubic'
    $g.SmoothingMode = 'HighQuality'
    $g.PixelOffsetMode = 'HighQuality'
    $g.DrawImage($src, 0, 0, $s, $s)
    $ms = New-Object System.IO.MemoryStream
    $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $entries += , $ms.ToArray()
    $g.Dispose(); $bmp.Dispose()
}
$src.Dispose()

$out = New-Object System.IO.MemoryStream
$w = New-Object System.IO.BinaryWriter $out
$w.Write([UInt16]0)              # reserved
$w.Write([UInt16]1)              # type: icon
$w.Write([UInt16]$sizes.Count)
$offset = 6 + 16 * $sizes.Count
for ($i = 0; $i -lt $sizes.Count; $i++) {
    $s = $sizes[$i]
    $dim = if ($s -ge 256) { 0 } else { $s }   # 256 은 0 으로 표기
    $w.Write([byte]$dim); $w.Write([byte]$dim)
    $w.Write([byte]0); $w.Write([byte]0)        # 팔레트 없음, reserved
    $w.Write([UInt16]1); $w.Write([UInt16]32)   # planes, bpp
    $w.Write([UInt32]$entries[$i].Length)
    $w.Write([UInt32]$offset)
    $offset += $entries[$i].Length
}
foreach ($e in $entries) { $w.Write($e) }
$w.Flush()
[System.IO.File]::WriteAllBytes($ico, $out.ToArray())

# 2) ICO → .syso (첫 번째 아이콘 리소스가 탐색기·작업 표시줄 아이콘이 된다)
Set-Content -Path $rc -Value '1 ICON "icon.ico"' -Encoding Ascii
Push-Location $PSScriptRoot
try {
    & windres --input xmview.rc --output-format=coff --output $syso
    if ($LASTEXITCODE -ne 0) { throw "windres failed ($LASTEXITCODE)" }
} finally {
    Pop-Location
}
Write-Host "created: $syso"
