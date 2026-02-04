# k8s-exposer Shell Completion Guide

## Bash Completion

### Temporary (current session only)
```bash
source <(k8s-exposer completion bash)
```

### Permanent Installation

**Linux:**
```bash
k8s-exposer completion bash > /etc/bash_completion.d/k8s-exposer
```

**macOS:**
```bash
k8s-exposer completion bash > $(brew --prefix)/etc/bash_completion.d/k8s-exposer
```

**User-local (Linux):**
```bash
mkdir -p ~/.local/share/bash-completion/completions
k8s-exposer completion bash > ~/.local/share/bash-completion/completions/k8s-exposer
```

### Requirements
- `bash-completion` package must be installed
- Install on Debian/Ubuntu: `apt install bash-completion`

## Zsh Completion

### Temporary (current session only)
```zsh
source <(k8s-exposer completion zsh)
```

### Permanent Installation

**Method 1: Oh-My-Zsh**
```bash
mkdir -p ~/.oh-my-zsh/completions
k8s-exposer completion zsh > ~/.oh-my-zsh/completions/_k8s-exposer
```

**Method 2: System-wide**
```bash
k8s-exposer completion zsh > /usr/local/share/zsh/site-functions/_k8s-exposer
```

**Method 3: fpath**
```bash
# Add to ~/.zshrc BEFORE compinit:
fpath=(~/.zsh/completion $fpath)
mkdir -p ~/.zsh/completion
k8s-exposer completion zsh > ~/.zsh/completion/_k8s-exposer
```

### Enable completions (add to ~/.zshrc if not present)
```bash
autoload -Uz compinit
compinit
```

## Fish Completion

### Temporary (current session only)
```fish
k8s-exposer completion fish | source
```

### Permanent Installation
```bash
k8s-exposer completion fish > ~/.config/fish/completions/k8s-exposer.fish
```

## PowerShell Completion

### Temporary (current session only)
```powershell
k8s-exposer completion powershell | Out-String | Invoke-Expression
```

### Permanent Installation

Add to PowerShell profile (`$PROFILE`):
```powershell
k8s-exposer completion powershell | Out-String | Invoke-Expression
```

## Testing Completion

After installation, start a new shell and test:

```bash
# Type and press TAB
k8s-exposer [TAB]
# Should show: completion, help, metrics, services, status, sync, version

k8s-exposer s[TAB]
# Should show: services, status, sync

k8s-exposer services [TAB]
# Should show: get, list

k8s-exposer --[TAB]
# Should show: --help, --json, --server
```

## Completion Features

The CLI supports intelligent completions for:

### Commands
- All main commands: `services`, `status`, `metrics`, `sync`, `version`
- Subcommands: `services list`, `services get`

### Flags
- Global flags: `--server`, `--json`, `--help`
- Command-specific flags

### Dynamic Completions (future)
- Service names for `k8s-exposer services get <TAB>`
- Server URLs from config file

## Troubleshooting

### Bash completion not working

1. Check if bash-completion is installed:
   ```bash
   dpkg -l | grep bash-completion  # Debian/Ubuntu
   rpm -qa | grep bash-completion  # RHEL/CentOS
   ```

2. Verify completion script:
   ```bash
   ls -l /etc/bash_completion.d/k8s-exposer
   ```

3. Source completion manually:
   ```bash
   source /etc/bash_completion.d/k8s-exposer
   ```

4. Check if completion is loaded:
   ```bash
   complete -p | grep k8s-exposer
   ```

### Zsh completion not working

1. Check fpath:
   ```zsh
   echo $fpath
   ```

2. Verify compinit is called:
   ```zsh
   grep compinit ~/.zshrc
   ```

3. Rebuild completion cache:
   ```zsh
   rm -f ~/.zcompdump
   compinit
   ```

### Completion shows errors

1. Regenerate completion script
2. Ensure CLI binary is in PATH
3. Check for conflicting completions

## Advanced Usage

### Disable completion descriptions
```bash
k8s-exposer completion bash --no-descriptions > /etc/bash_completion.d/k8s-exposer
```

### Debug completions
```bash
# Set debug file
export BASH_COMP_DEBUG_FILE=/tmp/completion-debug.log

# Use completion
k8s-exposer [TAB]

# Check debug output
cat /tmp/completion-debug.log
```

## Installation Script

Create a helper script for easy installation:

```bash
#!/bin/bash
# install-completion.sh

SHELL_NAME="${1:-bash}"

case "$SHELL_NAME" in
  bash)
    if command -v k8s-exposer &>/dev/null; then
      k8s-exposer completion bash > /etc/bash_completion.d/k8s-exposer
      echo "✓ Bash completion installed"
    fi
    ;;
  zsh)
    if command -v k8s-exposer &>/dev/null; then
      mkdir -p ~/.zsh/completion
      k8s-exposer completion zsh > ~/.zsh/completion/_k8s-exposer
      echo "✓ Zsh completion installed"
      echo "  Add to ~/.zshrc before compinit:"
      echo "    fpath=(~/.zsh/completion \$fpath)"
    fi
    ;;
  fish)
    if command -v k8s-exposer &>/dev/null; then
      mkdir -p ~/.config/fish/completions
      k8s-exposer completion fish > ~/.config/fish/completions/k8s-exposer.fish
      echo "✓ Fish completion installed"
    fi
    ;;
  *)
    echo "Usage: $0 {bash|zsh|fish}"
    exit 1
    ;;
esac
```

Usage:
```bash
chmod +x install-completion.sh
sudo ./install-completion.sh bash
```
