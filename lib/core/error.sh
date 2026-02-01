#!/bin/bash

# Error handling and logging module
# Provides error trapping, logging, and cleanup functionality

LOG_FILE=""

# Initialize error handling and logging
# Sets up traps for ERR and EXIT, creates log directory and file
error_init() {
    set -euo pipefail
    trap 'error_handler $? $LINENO' ERR
    trap 'error_cleanup' EXIT
    
    mkdir -p ~/.openboot/logs
    LOG_FILE=~/.openboot/logs/install-$(date +%Y%m%d-%H%M%S).log
    export LOG_FILE
    
    error_log "INFO" "Installation started"
}

# Log a message to file and optionally to stderr
# Arguments:
#   $1 - Log level (INFO, WARN, ERROR)
#   $2 - Message to log
error_log() {
    local level="$1"
    local message="$2"
    local timestamp
    timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    
    echo "[$timestamp] [$level] $message" >> "$LOG_FILE"
    
    if [[ "$level" == "ERROR" ]]; then
        echo "[$level] $message" >&2
    fi
}

# Handle trapped errors
# Arguments:
#   $1 - Exit code from failed command
#   $2 - Line number where error occurred
error_handler() {
    local exit_code=$1
    local line=$2
    error_log "ERROR" "Failed at line $line with exit code $exit_code"
    echo ""
    echo "Installation failed. See log: $LOG_FILE"
    echo "To retry: ./install.sh --resume"
}

# Cleanup on exit (success or failure)
# Logs final status based on exit code
error_cleanup() {
    local exit_code=$?
    if [[ $exit_code -eq 0 ]]; then
        error_log "INFO" "Installation completed successfully"
    fi
}
