@echo off
rem Windows: fire dev, fire validate, fire search ...
cd /d "%~dp0"
go build -C server -o ..\bin\fire.exe . || exit /b 1
bin\fire.exe %*
