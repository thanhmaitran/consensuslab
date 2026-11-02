# Windows equivalent of `make all`: format check, vet, tests, build.
$ErrorActionPreference = "Stop"
$unformatted = gofmt -l .
if ($unformatted) { Write-Host "gofmt needed on:`n$unformatted"; exit 1 }
go vet ./...
go test ./...
New-Item -ItemType Directory -Force bin | Out-Null
go build -o bin/consensuslab.exe ./cmd/consensuslab
Write-Host "OK"
