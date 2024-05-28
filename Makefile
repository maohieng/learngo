run:
	go run channel/main.go -w true -f 19s.wav

buildwin:
	GOOS=windows GOARCH=amd64 go build -o bin/channel.exe channel/main.go

buildlinux:
	GOOS=linux GOARCH=amd64 go build -o bin/channel_linux channel/main.go

buildmac:
	GOOS=darwin GOARCH=amd64 go build -o bin/channel_mac channel/main.go