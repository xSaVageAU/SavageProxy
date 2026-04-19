@echo off

echo Building for Windows...
set GOOS=windows
set GOARCH=amd64
go build -v -o savage-proxy.exe main.go

echo.
echo Building for Linux...
set GOOS=linux
set GOARCH=amd64
go build -v -o savage-proxy-linux main.go

echo.
echo Build process finished.
pause
