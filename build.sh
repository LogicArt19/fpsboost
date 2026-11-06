#!/bin/sh
# Compile FPSBoost.exe (Windows 64 bits) depuis Linux
set -e
cd "$(dirname "$0")/src"
llvm-rc -FO app.res app.rc
llvm-cvtres -MACHINE:X64 -OUT:rsrc_windows_amd64.syso app.res
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-H windowsgui -s -w" -o ../release/FPSBoost.exe .
rm -f app.res rsrc_windows_amd64.syso
