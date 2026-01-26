.PHONY: all help build run test install clean

BIN_NAME=hexdm
INSTALL_DIR=/usr/local/bin
PAM_DIR=/etc/pam.d
SYSTEMD_DIR=/etc/systemd/system

all: help

help:
	@echo "HexDM"
	@echo "Hex Display Manager is a simple TUI driven display manager written in Go for qtile/wayland systems."
	@echo ""
	@echo "USAGE: "
	@echo "	make build          : Compiles the binary to ./bin/hex"
	@echo "	sudo make install   : Installs binary, PAM configs, and systemd service"
	@echo "	make clean          : Removes build artifacts"
	@echo "	sudo make run       : Runs locally for testing (requires sudo for PAM)"

build:
	@mkdir -p bin/
	@echo "Building..."
	CGO_ENABLED=1 go build -o bin/hex cmd/main.go
	@echo "Build complete: bin/hex"

run:
	@if [ ! -f /etc/pam.d/hexdm ]; then \
		echo "Error: PAM config missing. Run 'sudo make install' first."; \
		exit 1; \
	fi
	@echo "Running HexDM (Development Mode)..."
	@mkdir -p logs/
	-go run cmd/main.go 2> logs/crash.log
	@echo "--- CRASH LOG ---"
	@cat logs/crash.log

install: build
	@if [ "$$(id -u)" -ne 0 ]; then echo "Must be root. (sudo make install)"; exit 1; fi
	
	@echo "Installing Binary..."
	@cp bin/hex $(INSTALL_DIR)/$(BIN_NAME)
	@chmod +x $(INSTALL_DIR)/$(BIN_NAME)
	@echo "done."
	
	@echo "Installing Configs..."
	@cp config/hexdm $(PAM_DIR)/hexdm
	@cp config/hexdm_nofprint $(PAM_DIR)/hexdm_nofprint
	@echo "done."
	
	@echo "Installing Systemd Service..."
	@cp config/hexdm.service $(SYSTEMD_DIR)/hexdm.service
	@echo "done."
	
	@echo "Enabling Service..."
	@systemctl daemon-reload
	@systemctl enable hexdm
	@systemctl set-default graphical.target
	@echo "done."
	
	@echo ""
	@echo "--- INSTALLATION COMPLETE ---"
	@echo "Finished. Reboot to start HexDM."

clean:
	@echo "Cleaning..."
	@rm -rf bin/ logs/
	@go fmt ./...
