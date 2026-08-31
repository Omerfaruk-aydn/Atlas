package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/crush/internal/clipboard"
	"github.com/charmbracelet/crush/internal/ui/common"
	"github.com/spf13/cobra"
)

// clipcheckCmd reports what the clipboard actually holds. Pasting an image
// depends on two independent things — the keystroke reaching the application
// and the clipboard yielding something usable — and when a paste does nothing
// there is no way to tell which half failed. This settles the second half on
// its own, with no keybinding involved.
var clipcheckCmd = &cobra.Command{
	Use:    "clipcheck",
	Short:  "Report what the system clipboard currently holds",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		if err := clipboard.Init(); err != nil {
			fmt.Fprintf(out, "clipboard init failed: %v\n", err)
			return nil
		}

		fmt.Fprintln(out, "image data  :")
		if data, err := clipboard.Read(clipboard.FormatImage); err != nil {
			fmt.Fprintf(out, "   none (%v)\n", err)
		} else {
			fmt.Fprintf(out, "   %d bytes -> would attach as paste_N.png\n", len(data))
		}

		fmt.Fprintln(out, "file list   :")
		paths, err := clipboard.ReadFiles()
		if err != nil {
			fmt.Fprintf(out, "   none (%v)\n", err)
		}
		for _, p := range paths {
			note := "not a supported image type"
			if isAllowedImage(p) {
				note = "OK"
				if info, statErr := os.Stat(p); statErr != nil {
					note = fmt.Sprintf("unreadable: %v", statErr)
				} else if info.IsDir() {
					note = "a folder, not a file"
				} else if info.Size() > common.MaxAttachmentSize {
					note = fmt.Sprintf("too large: %d bytes, max %d", info.Size(), common.MaxAttachmentSize)
				}
			}
			fmt.Fprintf(out, "   %s  [%s]\n", p, note)
		}

		fmt.Fprintln(out, "text        :")
		if data, err := clipboard.Read(clipboard.FormatText); err != nil {
			fmt.Fprintf(out, "   none (%v)\n", err)
		} else {
			text := strings.TrimSpace(string(data))
			if i := strings.IndexAny(text, "\r\n"); i >= 0 {
				text = text[:i] + " …"
			}
			fmt.Fprintf(out, "   %q\n", text)
		}

		fmt.Fprintln(out)
		switch {
		case len(paths) > 0 && isAllowedImage(paths[0]):
			fmt.Fprintf(out, "=> pasting would attach %s\n", filepath.Base(paths[0]))
		case len(paths) > 0:
			fmt.Fprintln(out, "=> the clipboard holds files, but none is a .png/.jpg/.jpeg")
		default:
			fmt.Fprintln(out, "=> no file list on the clipboard; copy an image file in Explorer first")
		}
		return nil
	},
}

func isAllowedImage(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range common.AllowedImageTypes {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}
