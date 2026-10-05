<#
.SYNOPSIS
  Installs the skillgild CLI / MCP server on Windows from GitHub Releases.

.DESCRIPTION
  irm https://raw.githubusercontent.com/SkillGild/skillgild/main/scripts/install-cli.ps1 | iex

  Environment variables:
    SKILLGILD_VERSION      release to install, e.g. v0.2.0 (default: latest)
    SKILLGILD_INSTALL_DIR  directory for skillgild.exe (default: %LOCALAPPDATA%\Programs\skillgild)
    SKILLGILD_REPO         GitHub repository that publishes releases (default: SkillGild/skillgild)

  The archive's SHA-256 is checked against the release's checksums.txt before anything
  is installed. The install directory is added to the user's PATH if it is missing.
#>
$ErrorActionPreference = "Stop"

$repo = if ($env:SKILLGILD_REPO) { $env:SKILLGILD_REPO } else { "SkillGild/skillgild" }
$version = if ($env:SKILLGILD_VERSION) { $env:SKILLGILD_VERSION } else { "latest" }
$installDir = if ($env:SKILLGILD_INSTALL_DIR) { $env:SKILLGILD_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\skillgild" }

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { throw "install-cli: unsupported CPU architecture $($env:PROCESSOR_ARCHITECTURE)" }
}

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
if ($version -eq "latest") {
  $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest" -Headers @{ "User-Agent" = "skillgild-install" }
  $version = $release.tag_name
}
if (-not $version.StartsWith("v")) { $version = "v$version" }
$plain = $version.Substring(1)

$archive = "skillgild_${plain}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$version"
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("skillgild-install-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Host "Downloading skillgild $version for windows/$arch"
  Invoke-WebRequest "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
  Invoke-WebRequest "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt") -UseBasicParsing

  $line = Get-Content (Join-Path $tmp "checksums.txt") | Where-Object { $_ -match "\s$([regex]::Escape($archive))$" } | Select-Object -First 1
  if (-not $line) { throw "install-cli: $archive is not listed in checksums.txt" }
  $expected = ($line -split "\s+")[0].ToLowerInvariant()
  $actual = (Get-FileHash (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLowerInvariant()
  if ($expected -ne $actual) { throw "install-cli: checksum mismatch for $archive; refusing to install" }

  Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force
  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  Copy-Item (Join-Path $tmp "skillgild.exe") (Join-Path $installDir "skillgild.exe") -Force
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

$exe = Join-Path $installDir "skillgild.exe"
Write-Host "Installed $(& $exe version) to $exe"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not (($userPath -split ";") -contains $installDir)) {
  [Environment]::SetEnvironmentVariable("Path", "$installDir;$userPath", "User")
  $env:Path = "$installDir;$env:Path"
  Write-Host "Added $installDir to your user PATH. Open a new terminal for other apps to see it."
}
Write-Host ""
Write-Host "Next: skillgild login"
Write-Host "Then register the MCP server with your agent, e.g.:"
Write-Host "  claude mcp add --scope user skillgild -- `"$exe`" mcp"
