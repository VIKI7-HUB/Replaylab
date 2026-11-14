$p = (Resolve-Path "LLM-Reliability_Walkthrough.pptx").Path
$out = Join-Path (Get-Location) "LLM-Reliability_Walkthrough_preview.mp4"
$app = New-Object -ComObject PowerPoint.Application
$pr = $app.Presentations.Open($p, $true, $false, $false)
$pr.CreateVideo($out, $false, 4, 720, 30, 85)
while ($pr.CreateVideoStatus -eq 1 -or $pr.CreateVideoStatus -eq 0) { Start-Sleep -Seconds 3 }
"status=$($pr.CreateVideoStatus)"
$pr.Close(); $app.Quit()
