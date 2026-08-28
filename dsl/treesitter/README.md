# Syntax Highlighting for TreeSitter

This is an AI generated `grammar.js` for Treesitter. This will allow syntax highlighting for vim.

Hexcript is the greeter configuration language for HexDM. Source files use the `.hc` or `.hexcript`
extensions.

## Setup Instructions

### LazyVim

Create `~/.config/nvim/lua/plugins/hexcript.lua`:

```lua
return {
  {
    "nvim-treesitter/nvim-treesitter",
    opts = { ensure_installed = { "hexcript" } },
    init = function()
      -- Neovim has no built-in rule for .hc, so add one.
      vim.filetype.add({ extension = { hc = "hexcript" } })

      -- Teach nvim-treesitter where the parser lives. This must be registered
      -- before TSUpdate fires, which is why it goes in `init` rather than `opts`.
      vim.api.nvim_create_autocmd("User", {
        pattern = "TSUpdate",
        callback = function()
          require("nvim-treesitter.parsers").hexcript = {
            install_info = {
              url = "https://github.com/tesinclair/HexDM",
              location = "dsl/treesitter",
              queries = "queries",
            },
          }
        end,
      })

      -- The main branch does not enable highlighting for you.
      vim.api.nvim_create_autocmd("FileType", {
        pattern = "hexcript",
        callback = function()
          vim.treesitter.start()
        end,
      })
    end,
  },
}
```

Then restart Neovim and run:

```
:TSUpdate
:TSInstall hexcript
```


## Requirements

- Neovim 0.12.0 or later
- `nvim-treesitter` on the `main` branch (what LazyVim ships)
- `tree-sitter` CLI 0.26.1 or later, installed via your **system package manager**, not npm
- A C compiler on your `PATH`
- `git`, `tar` and `curl` on your `PATH`

On Arch:

```sh
sudo pacman -S tree-sitter-cli base-devel
```

