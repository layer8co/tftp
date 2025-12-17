run:
	go run cmd/tftp.go -r -f ./cmd/gopher.png -l 127.0.0.1:2000

writerun:
	go run cmd/tftp.go -f ./cmd/gopher.png -w -r -l 127.0.0.1:2000


test: 
	go test ./...


build:
	go build -o tftp.out ./cmd/tftp.go 
