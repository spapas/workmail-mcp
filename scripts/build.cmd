@echo off
setlocal
if not exist bin mkdir bin
go build -trimpath -o bin\workmail-mcp.exe .\cmd\workmail
if errorlevel 1 exit /b %errorlevel%
echo Built bin\workmail-mcp.exe
