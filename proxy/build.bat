@echo off

echo Building for Windows...
set GOOS=windows
set GOARCH=amd64
go build -v -o savage-proxy.exe ./cmd/savage-proxy

echo.
echo Building for Linux...
set GOOS=linux
set GOARCH=amd64
go build -v -o savage-proxy-linux ./cmd/savage-proxy

echo.
echo Build process finished.
pause
