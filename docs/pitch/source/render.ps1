param([string]$Pptx, [string]$OutDir)
$Pptx = (Resolve-Path $Pptx).Path
New-Item -ItemType Directory -Force $OutDir | Out-Null
$OutDir = (Resolve-Path $OutDir).Path
Get-ChildItem $OutDir -Filter *.png | Remove-Item -Force
$app = New-Object -ComObject PowerPoint.Application
$pres = $app.Presentations.Open($Pptx, $true, $false, $false)
$i = 1
foreach ($s in $pres.Slides) { $s.Export((Join-Path $OutDir ("s{0:D2}.png" -f $i)), "PNG", 1600, 900); $i++ }
$pres.Close()
$app.Quit()
Write-Output "rendered $($i-1) slides to $OutDir"
