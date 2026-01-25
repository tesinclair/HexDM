.PHONY: all hello build run test
all: hello build

hello:
	@echo "Hex Display Manager is a simple TUI driven display manager written in Go for qtile/wayland systems."

build:
	mkdir -p bin/
	@echo "Building..."
	CGO_ENABLED=true go build -o bin/hex cmd/main.go


run:
	@echo "Running..."
	mkdir -p logs/
	-go run cmd/main.go 2> logs/crash.log; printf "\nStatus: $$?\n" >> logs/crash.log
	cat logs/crash.log


test:
	@echo "Not Implemented."
