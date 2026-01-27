.PHONY: all help build run test install clean

BIN_NAME=hexdm
HELPER_BIN_NAME=hex-auth

INSTALL_DIR=/usr/local/bin
PAM_DIR=/etc/pam.d
SYSTEMD_DIR=/etc/systemd/system
HELPER_DIR=/usr/libexec/hexdm

all: help

help:
	@echo "HexDM"
	@echo "Hex Display Manager is a simple TUI driven display manager written in Go for qtile/wayland systems."
	@echo ""
	@echo "NORMAL USAGE:"
	@echo " sudo make install   : Compiles and installs the program"
	@echo ""
	@echo "SPECIAL USAGE: "
	@echo "	make build          : Compiles the binary to ./bin/hex"
	@echo "	sudo make install   : Installs binary, PAM configs, and systemd service"
	@echo "	make clean          : Removes build artifacts"
	@echo "	sudo make run       : Runs locally for testing (requires sudo for PAM)"

build_auth:
	@mkdir -p bin/
	@echo "Building Hex Authenticator..."
	CGO_ENABLED=1 go build -o bin/hex-auth cmd/hexdm-auth/main.go
	@echo "Build complete: bin/hex-auth"

build:
	@mkdir -p bin/
	@echo "Building Hex Display Manager"
	CGO_ENABLED=1 go build -o bin/hexdm cmd/hexdm/main.go
	@echo "Build complete: bin/hex"

run: build_auth
	@if [ ! -f /etc/pam.d/hexdm ]; then \
		echo "Error: PAM config missing. Run 'sudo make install' first."; \
		exit 1; \
	fi
	@echo "Running HexDM (Debug Mode)..."
	@mkdir -p logs/
	HEX_DEBUG=1 HEX_DRYRUN=1 HEX_FPRINT_PATH=bin/hex-auth go run cmd/hexdm/main.go 2> logs/crash.log
	@echo "--- CRASH LOG ---"
	@cat logs/crash.log

install: build build_auth
	@if [ "$$(id -u)" -ne 0 ]; then echo "Must be root. (sudo make install)"; exit 1; fi
	
	@echo "Installing Binaries..."
	install -D -m 755 bin/$(BIN_NAME) $(INSTALL_DIR)/$(BIN_NAME)
	install -D -m 755 bin/$(HELPER_BIN_NAME) $(HELPER_DIR)/$(HELPER_BIN_NAME)
	@echo "done."
	
	@echo "Installing Configs..."
	install -D -m 644 config/hexdm $(PAM_DIR)/hexdm
	install -D -m 644 config/hexdm_nofprint $(PAM_DIR)/hexdm_nofprint
	install -D -m 644 config/hexdm.service $(SYSTEMD_DIR)/hexdm.service
	@echo "done."
	
	@echo "Installing Systemd Service..."
	@cp config/hexdm.service $(SYSTEMD_DIR)/hexdm.service
	@echo "done."
	
	@echo "Enabling Service..."
	systemctl daemon-reload
	systemctl enable hexdm
	@systemctl set-default graphical.target
	@echo "done."
	
	@echo ""
	@echo "--- INSTALLATION COMPLETE ---"
	@echo "Finished. Reboot to start HexDM."

clean:
	@echo "Cleaning..."
	@rm -rf bin/ logs/
	@go clean
