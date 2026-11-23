param(
    [ValidateSet("all", "lint", "test", "wasm")]
    [string]$Target = "all",
    [string]$ClientId = "",
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"

function Invoke-Lint {
    Write-Host "==> go fix -diff ./..."
    $diff = go fix -diff ./...
    if ($LASTEXITCODE -ne 0) { throw "go fix failed" }
    if ($diff) {
        Write-Host $diff
        throw "go fix reported suggested changes"
    }
    Write-Host "==> go vet ./..."
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed" }
    if (Get-Command golangci-lint-v2 -ErrorAction SilentlyContinue) {
        Write-Host "==> golangci-lint-v2 run ./..."
        golangci-lint-v2 run ./...
        if ($LASTEXITCODE -ne 0) { throw "golangci-lint-v2 failed" }
    }
}

function Invoke-Test {
    Write-Host "==> go test -count=1 -v ./..."
    go test -count=1 -v ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed" }
}

function Invoke-WasmBuild {
    New-Item -ItemType Directory -Force -Path "build\bin" | Out-Null
    $ldflags = "-s -w -X github.com/mrpoundsign/PoundControl-Twitch/internal/twitch.PluginVersion=$Version"
    if ($ClientId -ne "") {
        $ldflags += " -X github.com/mrpoundsign/PoundControl-Twitch/internal/twitch.DefaultClientID=$ClientId"
    }

    Write-Host "==> Building build/bin/poundcontrol-twitch.wasm (wasip1/wasm reactor)"
    $prevGOOS = $env:GOOS
    $prevGOARCH = $env:GOARCH
    try {
        $env:GOOS = "wasip1"
        $env:GOARCH = "wasm"
        go build -buildmode=c-shared -ldflags $ldflags -o "build\bin\poundcontrol-twitch.wasm" .
        if ($LASTEXITCODE -ne 0) { throw "wasm build failed" }
    } finally {
        $env:GOOS = $prevGOOS
        $env:GOARCH = $prevGOARCH
    }
    $item = Get-Item "build\bin\poundcontrol-twitch.wasm"
    Write-Host ("Built {0} ({1:N0} bytes)" -f $item.FullName, $item.Length)
}

switch ($Target) {
    "lint" { Invoke-Lint }
    "test" { Invoke-Test }
    "wasm" { Invoke-WasmBuild }
    "all"  {
        Invoke-Lint
        Invoke-Test
        Invoke-WasmBuild
    }
}
