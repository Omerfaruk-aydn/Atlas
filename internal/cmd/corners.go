package cmd

import (
	"fmt"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/styles"
	"github.com/spf13/cobra"
)

// cornersCmd prints a sample box in every available corner style so the user
// can see which ones their terminal font actually carries. Whether a style
// renders is a property of the font, not of ATLAS-AGENT: a font missing a
// glyph draws a replacement box in its place. Printing the samples from the
// binary itself means they go through the same terminal and font as the TUI,
// which a snippet pasted into a different shell would not guarantee.
var cornersCmd = &cobra.Command{
	Use:   "corners",
	Short: "Preview the box corner styles your terminal font supports",
	Long: "Print a sample frame in each available corner style.\n\n" +
		"Styles whose corners show as an empty box are missing from your\n" +
		"terminal font. Pick one that renders cleanly and set it in your\n" +
		"config as options.tui.box_corners.",
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		const label = "sample"

		for _, style := range styles.BoxCornerStyles() {
			styles.SetBoxCorners(style)

			width := 34
			rule := strings.Repeat(styles.BoxHorizontal, width-len(label)-4)
			fmt.Fprintf(out, "\n  %s\n", style)
			fmt.Fprintf(out, "    %s%s %s %s%s\n",
				styles.BoxTopLeft, styles.BoxHorizontal, label, rule, styles.BoxTopRight)
			fmt.Fprintf(out, "    %s%s%s\n",
				styles.BoxVertical, strings.Repeat(" ", width-2), styles.BoxVertical)
			fmt.Fprintf(out, "    %s%s%s\n",
				styles.BoxBottomLeft, strings.Repeat(styles.BoxHorizontal, width-2), styles.BoxBottomRight)
		}

		detected := styles.DetectCornerStyle()
		fmt.Fprintf(out, "\n  Corners showing as an empty box are missing from your font.\n")
		fmt.Fprintf(out, "  ATLAS-AGENT picked %q for this terminal. To choose another:\n\n", detected)
		fmt.Fprintf(out, "    { \"options\": { \"tui\": { \"box_corners\": %q } } }\n\n", detected)
		return nil
	},
}
