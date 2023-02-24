set VERSION="0.9.5"

env GOOS=windows GOARCH=amd64 go build -pgo=auto -ldflags="-s -w -X 'main.Version=%VERSION%' -X 'main.BuildTime=%DATE%'" -o f1gopher-cmdline.exe

timeout /t 2

powershell Compress-Archive -Force f1gopher-cmdline.exe f1gopher-cmdline-win-amd64.zip

