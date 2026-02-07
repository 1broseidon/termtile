package config

// BuiltinLayouts returns the built-in layout library.
//
// These are always available to users without needing to define them in YAML.
// Users can define additional custom layouts in their config file.
func BuiltinLayouts() map[string]Layout {
	return map[string]Layout{
		"grid": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
			FlexibleLastRow: true,
		},
		"columns": {
			Mode: LayoutModeVertical,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
		},
		"rows": {
			Mode: LayoutModeHorizontal,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
		},
		"half-left": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionLeftHalf,
			},
			FlexibleLastRow: true,
		},
		"half-right": {
			Mode: LayoutModeAuto,
			TileRegion: TileRegion{
				Type: RegionRightHalf,
			},
			FlexibleLastRow: true,
		},
		"master-stack": {
			Mode: LayoutModeMasterStack,
			TileRegion: TileRegion{
				Type: RegionFull,
			},
			MasterStack: MasterStack{
				MasterWidthPercent: 40,
				MaxStackRows:       3,
				MaxStackCols:       2,
			},
		},
	}
}
