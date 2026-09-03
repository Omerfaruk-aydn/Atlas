Read a file by path with line numbers; supports offset and line limit (default {{ .DefaultReadLimit }}, max {{ .MaxViewSizeKB }}KB returned file content section); renders images (PNG, JPEG, GIF, WebP); use ls for directories.
{{ if .HashAnchors }}
Each line is shown as `line|hash|content`. For a single-line change, pass that hash as edit's anchor_hash with anchor_line instead of old_string -- no need to reproduce the line's exact text or whitespace. A stale hash is rejected, telling you to re-read the file rather than silently landing on the wrong line.
{{ end }}