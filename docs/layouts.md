# Tiling Layouts

termtile provides several tiling algorithms and supports restricted regions for window placement.

## Layout Modes

| Mode | Description |
|---|---|
| **Auto** | Dynamic grid calculation. `cols = ceil(sqrt(n))`, `rows = ceil(n/cols)`. |
| **Fixed** | Explicit rows and columns grid (e.g., 2x2). |
| **Vertical** | Single column; windows stacked top-to-bottom. |
| **Horizontal** | Single row; windows placed side-by-side. |
| **Master-Stack** | One master window on the left, others in a grid on the right. |

## Built-in Layouts

termtile comes with several pre-configured layouts:

| Name | Mode | Region | Notes |
|---|---|---|---|
| `grid` | Auto | Full | Optimized dynamic grid. |
| `columns` | Vertical | Full | Simple column stack. |
| `rows` | Horizontal | Full | Side-by-side row. |
| `half-left` | Auto | Left Half | Tiles only on the left 50% of screen. |
| `half-right` | Auto | Right Half | Tiles only on the right 50% of screen. |
| `master-stack` | MasterStack | Full | Slot 0 is the master (40% width). |

## Tile Regions

Regions restrict where windows are placed on the screen.

- `full`: Entire work area (minus docks/panels).
- `left-half` / `right-half`: Vertical split.
- `top-half` / `bottom-half`: Horizontal split.
- `custom`: Percentage-based custom area.

```yaml
tile_region:
  type: "custom"
  x_percent: 10
  y_percent: 10
  width_percent: 80
  height_percent: 80
```

## Customization

### Gaps and Padding
- **Gap Size**: Space between windows.
- **Screen Padding**: Extra space around the edges of the monitor (top, bottom, left, right).

### Constraints
- **Max Terminal Size**: Caps the width or height of individual windows in a layout.
- **Flexible Last Row**: In `auto` mode, the last row can expand to fill the width if it has fewer windows than columns.

### Terminal Sorting
Determines the order windows are placed into the grid:
- `position`: Sorted by Y then X coordinates.
- `window_id`: Sorted by X11 window ID.
- `active_first`: The focused window is placed in slot 0.
- `session_slot`: Sorted by tmux slot number (ideal for Master-Stack).

## Interaction

### Cycling Layouts
You can cycle through layouts using hotkeys (default `Mod4-Mod1-]` and `Mod4-Mod1-[`).

### Undo
Restores windows to their exact positions before the last tiling operation.

### Preview
Temporarily apply a layout to see how it looks:
```bash
termtile layout preview --duration 5 grid
```
After 5 seconds, the windows will revert to their previous positions.
