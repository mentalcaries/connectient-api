# Theme extraction service boundary

Logo palette extraction remains in the Next.js application as a narrowly scoped
authenticated image-processing adapter. Go does not expose a duplicate
`/settings/extract-theme` endpoint.

This preserves the current ColorThief/MMCQ palette selection and ordering used by
the UI. A generic Go quantizer could produce visibly different themes even with the
same 20-colour count and HSL filters.

The retained adapter must continue to:

- require settings permission;
- read only the current practice logo;
- accept only the configured R2 public origin and owned practice key;
- constrain redirects, fetch time, response bytes, MIME type, and decoded pixels;
- filter colours to lightness at most 85% and saturation at least 8%;
- return 400 `insufficient_colors` when fewer than three colours remain.

If extraction later moves to Go, port a ColorThief-compatible MMCQ implementation
and verify it against cross-language image fixtures before cutover. No database
migration or Go application code is required for this decision.
