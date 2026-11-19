#!/bin/bash

set -euxo pipefail

pwsh -NoProfile -Command 'Set-PSRepository PSGallery -InstallationPolicy Trusted; Install-Module PSScriptAnalyzer -RequiredVersion 1.22.0 -Scope CurrentUser -Force'
powershell.exe -NoProfile -Command 'Install-PackageProvider NuGet -MinimumVersion 2.8.5.201 -Scope CurrentUser -Force | Out-Null; Install-Module Pester -RequiredVersion 6.2.0 -Scope CurrentUser -Force -SkipPublisherCheck'

make init
make generate-check
make check

if [[ $GITHUB_REF == refs/tags/v* ]]; then
  ln -s /c/Users/runneradmin/.gnupg ~/.gnupg
  make release
else
  make release-snapshot
fi
