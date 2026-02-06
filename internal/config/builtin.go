package config

// BuiltinLayouts returns the built-in layout library.
//
// These are always available to users without needing to define them in YAML.
func BuiltinLayouts() map[string]Layout {
	return map[string]Layout{
		"auto_full": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"vertical_full": {
			Mode: LayoutModeVertical,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"horizontal_full": {
			Mode: LayoutModeHorizontal,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"left_half_auto": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionLeftHalf,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"right_half_auto": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionRightHalf,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"top_half_auto": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionTopHalf,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"bottom_half_auto": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionBottomHalf,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
		"fixed_2x2_full": {
			Mode: LayoutModeFixed,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
			FixedGrid: FixedGrid{
				Rows: 2,
				Cols: 2,
			},
			MaxTerminalWidth:  0,
			MaxTerminalHeight: 0,
		},
	}
}
