# OpenBoot Demo (~18s)
#
# Prerequisites:
#   go build -ldflags "-X github.com/openbootdotdev/openboot/internal/cli.version=0.20.0" \
#     -o /tmp/openboot-demo ./cmd/openboot
#   cp scripts/demo-wrapper.sh /tmp/openboot-demo-wrapper.sh
#   chmod +x /tmp/openboot-demo-wrapper.sh
#
# Record: vhs demo.tape

Output demo.gif
Output demo.webm

Set Shell "bash"
Set FontSize 26
Set Width 1600
Set Height 960
Set Padding 24
Set Framerate 60
Set Theme "Catppuccin Mocha"
Set TypingSpeed 45ms
Set CursorBlink false

Hide
Type "PS1='$ '"
Enter
Type "bash() { /tmp/openboot-demo-wrapper.sh; }"
Enter
Type "curl() { echo x; }"
Enter
Type "clear"
Enter
Show

# [0s] curl command
Sleep 300ms
Type "curl -fsSL openboot.dev/install | bash"
Sleep 600ms
Enter

# [2s] Wait for download simulation + openboot header + git auto-detect
Sleep 5.5s

# [7s] Preset: select "developer"
Down
Sleep 300ms
Enter
Sleep 1.5s

# [9s] Package Selector — jump straight to search
Sleep 800ms
Type "/"
Sleep 300ms
Type@80ms "docker"
Sleep 1.5s

# Toggle docker
Space
Sleep 600ms
Escape
Sleep 400ms

# [13s] Confirm
Enter
Sleep 1.2s
Enter
Sleep 1.5s

# [16s] Dry-run output + prompts fly by
Sleep 1s
Enter
Sleep 0.5s
Enter
Sleep 0.5s
Enter
Sleep 1s

# [19s] Completion — hold for 3s then end
Sleep 3s
