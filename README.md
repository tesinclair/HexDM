# HexDM
HexDM (Hex Display Manager) is a TUI based display manager build specifically for easy integration with fprintd (the fingerprint daemon for linux).
HexDM is named for the magic powered computer Hex at Unseen University in the Terry Pratchett discworld novels.

### Installation
**Prerequesites**: 
- Window manager: qtile
- Window protocol: Wayland
- OS: arch linux

To test the correct environment, run `qtile start -b wayland` from a tty, if this opens a new window manager, you are set up.
You can then run `qtile cmd-obj -o cmd -f shutdown` to kill the session.

**Installation**:
Installation is quite simple:

Navigate to your command line and run:
```
git clone github.com/tesinclair/HexDM
cd HexDM
sudo make install
```

With any luck, you should see a successful installation.
