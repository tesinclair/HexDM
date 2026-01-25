.PHONY: all hello build run test configure clean
all: hello build

hello:
	@echo "Hex Display Manager is a simple TUI driven display manager written in Go for qtile/wayland systems."

build:
	mkdir -p bin/
	@echo "Building..."
	CGO_ENABLED=true go build -o bin/hex cmd/main.go

run:
	@# Check /etc/pam.d/hexdm
	@if [ ! -f /etc/pam.d/hexdm ]; then \
		echo "hexdm pam config missing." \
		echo "Run <sudo make configure> to create it." \
		exit 1; \
	fi
	@echo "Running..."
	mkdir -p logs/
	-go run cmd/main.go 2> logs/crash.log; printf "\nStatus: $$?\n" >> logs/crash.log
	cat logs/crash.log

test:
	@echo "Not Implemented."

configure:
	@echo "Copying hexdm pam config to /etc/pam.d."
	cp config/hexdm config/hexdm_nofprint /etc/pam.d/
	@echo "Done."

clean:
	@echo "Cleaning bin and logs"
	-rm -rf logs bin
	@echo "Formatting go code"
	-go fmt ./...

