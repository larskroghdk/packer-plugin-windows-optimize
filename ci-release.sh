#!/bin/bash

set -euxo pipefail

pwsh -NoProfile -Command '$ErrorActionPreference = "Stop"; Set-PSRepository PSGallery -InstallationPolicy Trusted; Install-Module PSScriptAnalyzer -RequiredVersion 1.22.0 -Scope CurrentUser -Force; $modules = Join-Path ([Environment]::GetFolderPath("MyDocuments")) "WindowsPowerShell\Modules"; $null = New-Item -ItemType Directory -Force -Path $modules; Save-Module Pester -RequiredVersion 6.2.0 -Path $modules -Force'

make init
make generate-check
make check

if [[ $GITHUB_REF == refs/tags/v* ]]; then
  ln -s /c/Users/runneradmin/.gnupg ~/.gnupg
  make release
else
  make release-snapshot
fi
