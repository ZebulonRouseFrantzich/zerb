-- Full ZERB Configuration Example
--
-- This demonstrates all available configuration options.

zerb = {
  -- Optional metadata
  meta = {
    name = "My Development Environment",
    description = "Full-stack web development setup with Node, Python, and Rust",
  },

  -- Tools to install (via mise)
  tools = {
    -- Language runtimes with exact versions
    "node@20.11.0",
    "python@3.12.1",
    "rust@1.75.0",
    
    -- Tools from different backends
    "cargo:ripgrep",              -- From crates.io
    "npm:prettier",               -- From npm
    "ubi:sharkdp/bat",           -- Binary from GitHub releases
  },

  -- Configuration files to manage (via chezmoi)
  configs = {
    -- Simple file paths (as strings)
    "~/.zshrc",
    "~/.gitconfig",
    
    -- Directories (use table with recursive option)
    {
      path = "~/.config/nvim/",
      recursive = true,
    },
    
    -- Templated configs (chezmoi template processing)
    {
      path = "~/.config/alacritty/alacritty.yml",
      template = true,
    },
    
    -- Secret files (GPG encrypted)
    {
      path = "~/.ssh/config",
      template = true,
      secrets = true,
      private = true,  -- chmod 600
    },
  },

  -- Git repository for config versioning
  git = {
    remote = "https://github.com/username/dotfiles",
    branch = "main",
  },

  -- ZERB configuration options
  config = {
    backup_retention = 5,  -- Keep last 5 timestamped configs
  },
}
