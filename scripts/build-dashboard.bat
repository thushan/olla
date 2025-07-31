@echo off

REM Build the embedded dashboard for Olla

echo Building Olla Dashboard...

REM Change to dashboard directory
cd web\dashboard

REM Install dependencies if needed
if not exist "node_modules" (
    echo Installing dependencies...
    bun install
)

REM Build the dashboard
echo Building dashboard for production...
bun run build

REM Copy built files to embed location
echo Copying built files to embed location...
if exist "..\..\internal\app\handlers\dashboard\dist" rmdir /s /q "..\..\internal\app\handlers\dashboard\dist"
xcopy /E /I /Y dist ..\..\internal\app\handlers\dashboard\dist

cd ..\..

echo Dashboard build complete!
echo Now run 'make build' to compile Olla with the embedded dashboard.