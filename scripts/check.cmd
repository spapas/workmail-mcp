@echo off
setlocal

gofmt -w .
if errorlevel 1 exit /b %errorlevel%

go vet ./...
if errorlevel 1 exit /b %errorlevel%

go test ./...
if errorlevel 1 exit /b %errorlevel%

echo Checks passed.
