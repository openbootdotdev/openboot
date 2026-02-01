#!/usr/bin/env bash
# Dotfiles management module with backup and multi-format support

source "$(dirname "${BASH_SOURCE[0]}")/detect.sh"
source "$(dirname "${BASH_SOURCE[0]}")/error.sh"

DOTFILES_DIR="$HOME/.dotfiles"
BACKUP_DIR="$HOME/.openboot/backup"

# Clone dotfiles repo to ~/.dotfiles
# Args: repo (e.g., "user/dotfiles")
dotfiles_clone() {
    local repo="$1"
    
    if [[ -z "$repo" ]]; then
        error "Repository URL required"
        return 1
    fi
    
    if [[ -d "$DOTFILES_DIR" ]]; then
        echo "Dotfiles already exist at $DOTFILES_DIR"
        return 0
    fi
    
    git clone "https://github.com/$repo" "$DOTFILES_DIR" || {
        error "Failed to clone dotfiles repository"
        return 1
    }
    
    echo "Dotfiles cloned to $DOTFILES_DIR"
}

# Backup a single file to ~/.openboot/backup/
# Args: file (full path to file)
dotfiles_backup() {
    local file="$1"
    
    if [[ -z "$file" ]]; then
        error "File path required"
        return 1
    fi
    
    if [[ -f "$file" && ! -L "$file" ]]; then
        mkdir -p "$BACKUP_DIR" || {
            error "Failed to create backup directory"
            return 1
        }
        
        local basename
        basename=$(basename "$file")
        local timestamp
        timestamp=$(date +%s)
        
        cp "$file" "$BACKUP_DIR/${basename}.bak.${timestamp}" || {
            error "Failed to backup $file"
            return 1
        }
        
        echo "Backed up $file to $BACKUP_DIR/${basename}.bak.${timestamp}"
    fi
}

# Backup all existing dotfiles that would be overwritten
dotfiles_backup_existing() {
    if [[ ! -d "$DOTFILES_DIR" ]]; then
        error "Dotfiles directory not found at $DOTFILES_DIR"
        return 1
    fi
    
    if [[ -x "$DOTFILES_DIR/install.sh" ]]; then
        echo "Custom install script detected - manual backup recommended"
        return 0
    fi
    
    if [[ -d "$DOTFILES_DIR/zsh" ]] || [[ -d "$DOTFILES_DIR/git" ]] || [[ -d "$DOTFILES_DIR/vim" ]]; then
        echo "Stow structure detected - backing up potential conflicts"
        for dir in "$DOTFILES_DIR"/*/; do
            [[ -d "$dir" ]] || continue
            local name
            name=$(basename "$dir")
            
            [[ "$name" == ".git" ]] && continue
            
            for file in "$dir"/.* "$dir"/*; do
                [[ -e "$file" ]] || continue
                local basename
                basename=$(basename "$file")
                [[ "$basename" == "." || "$basename" == ".." ]] && continue
                
                local target="$HOME/$basename"
                dotfiles_backup "$target"
            done
        done
        return 0
    fi
    
    for file in "$DOTFILES_DIR"/.*; do
        [[ -f "$file" ]] || continue
        local basename
        basename=$(basename "$file")
        [[ "$basename" == ".git" || "$basename" == ".." || "$basename" == "." ]] && continue
        
        local target="$HOME/$basename"
        dotfiles_backup "$target"
    done
}

# Deploy dotfiles using stow, custom script, or direct symlinks
dotfiles_deploy() {
    if [[ ! -d "$DOTFILES_DIR" ]]; then
        error "Dotfiles directory not found at $DOTFILES_DIR"
        return 1
    fi
    
    # Priority 1: Custom install script
    if [[ -x "$DOTFILES_DIR/install.sh" ]]; then
        echo "Running custom install script..."
        (cd "$DOTFILES_DIR" && bash install.sh) || {
            error "Custom install script failed"
            return 1
        }
        echo "Custom install script completed"
        return 0
    fi
    
    # Priority 2: stow structure
    if [[ -d "$DOTFILES_DIR/zsh" ]] || [[ -d "$DOTFILES_DIR/git" ]] || [[ -d "$DOTFILES_DIR/vim" ]]; then
        echo "Stow structure detected - deploying with stow..."
        
        if ! command -v stow &> /dev/null; then
            error "stow not found - install with: brew install stow (macOS) or apt install stow (Linux)"
            return 1
        fi
        
        for dir in "$DOTFILES_DIR"/*/; do
            [[ -d "$dir" ]] || continue
            local name
            name=$(basename "$dir")
            
            [[ "$name" == ".git" ]] && continue
            
            echo "Stowing $name..."
            stow -v -d "$DOTFILES_DIR" -t "$HOME" "$name" || {
                error "Failed to stow $name"
                return 1
            }
        done
        
        echo "Stow deployment completed"
        return 0
    fi
    
    # Priority 3: Direct symlinks
    echo "Flat structure detected - creating direct symlinks..."
    for file in "$DOTFILES_DIR"/.*; do
        [[ -f "$file" ]] || continue
        local basename
        basename=$(basename "$file")
        [[ "$basename" == ".git" || "$basename" == ".." || "$basename" == "." ]] && continue
        
        local target="$HOME/$basename"
        
        if [[ -L "$target" ]]; then
            rm "$target"
        elif [[ -f "$target" ]]; then
            rm "$target"
        fi
        
        ln -sf "$file" "$target" || {
            error "Failed to symlink $file to $target"
            return 1
        }
        
        echo "Linked $basename"
    done
    
    echo "Direct symlink deployment completed"
}

# Replace {{NAME}}/{{EMAIL}} in a file
# Args: name, email, file
dotfiles_substitute_git() {
    local name="$1"
    local email="$2"
    local file="$3"
    
    if [[ -z "$name" || -z "$email" || -z "$file" ]]; then
        error "Name, email, and file path required"
        return 1
    fi
    
    if [[ ! -f "$file" ]]; then
        error "File not found: $file"
        return 1
    fi
    
    # Detect OS for sed compatibility
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS requires -i with empty string
        sed -i '' "s/{{NAME}}/$name/g" "$file" || {
            error "Failed to substitute NAME in $file"
            return 1
        }
        sed -i '' "s/{{EMAIL}}/$email/g" "$file" || {
            error "Failed to substitute EMAIL in $file"
            return 1
        }
    else
        # Linux sed
        sed -i "s/{{NAME}}/$name/g" "$file" || {
            error "Failed to substitute NAME in $file"
            return 1
        }
        sed -i "s/{{EMAIL}}/$email/g" "$file" || {
            error "Failed to substitute EMAIL in $file"
            return 1
        }
    fi
    
    echo "Substituted git info in $file"
}
