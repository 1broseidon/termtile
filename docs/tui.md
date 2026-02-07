# Interactive TUI

The termtile TUI (`termtile tui`) provides a visual way to manage your configuration and tiling behavior.

## Navigation

- **Tab / Shift-Tab**: Cycle through tabs.
- **1, 2, 3, 4**: Jump directly to a tab.
- **Up / Down**: Navigate lists.
- **Enter**: Select or toggle.
- **q / Ctrl+C**: Quit.

## Tabs

### 1. General Settings
Edit global options like `gap_size`, `default_layout`, and `preferred_terminal`. 
- Press `e` to enter edit mode for a field.
- Press `Esc` to cancel changes.

### 2. Layouts
Browse all available layouts with a live ASCII-art preview.
- **Preview**: Press `p` to see the layout applied to your actual windows for 5 seconds.
- **Apply**: Press `a` or `Enter` to switch to this layout immediately.
- **Default**: Press `d` to set this layout as your persistent default.

### 3. Agents
Manage your AI agent registry.
- View configured agents and those detected in your `PATH`.
- Press `D` to rescan your system for new agents.

### 4. Terminal Classes
Manage how termtile detects and spawns terminal emulators.
- **Add**: Press `a` to add a new class.
- **Delete**: Press `x` or `Delete` to remove a class.
- **Default**: Press `d` to toggle which class is the default for spawning.

## Saving Changes

termtile features a safe save system with a built-in diff viewer.

1. Press **Ctrl+S** to initiate a save.
2. A diff overlay will show exactly what lines will be added or removed from your `config.yaml`.
3. Press **Enter** to confirm and write to disk.
4. The TUI will automatically signal the daemon to reload the new configuration.
