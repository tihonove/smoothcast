# Upstreaming `--no-cursor` to wf-recorder

Goal: get rid of the `wf-recorder-nc` fork (see [lessons learned](lessons-learned.md) for why it exists).

State as of 2026-10-21 (`ammen99/wf-recorder`, 1.3k stars, release 0.6.0 —
October 2025):

- **The maintainer is active.** ammen99 merges himself; small PRs land in 1–14
  days (#344, #339, #337, #334, #318, #317, #315, etc.). Issues usually get
  closed within days. There are long-lived ones too (#256, #236 from
  2023–2024) — mostly contentious or unfinished.
- **Nobody has asked for the flag.** Searching issues/PRs (cursor, pointer, hide
  mouse, overlay_cursor, no-cursor) finds no requests. A cheap change with a
  clear motivation and no effect on default behavior is a good candidate.
- **The main risk is PR #347 (soreau, "Port to ext-copy-capture").** It drops
  wlr-screencopy entirely, and the cursor is hardcoded there too:
  `create_session(…, OPTIONS_PAINT_CURSORS)`. soreau is the second regular
  contributor (from the Wayfire team); the PR has been open since June, is
  referenced from #273 and waits for the wlroots 0.20 release. wlroots plans to
  remove wlr-screencopy altogether (wlroots MR 5149), so the future is
  ext-copy-capture.

**Plan:**

1. No need to scout with an issue first — go straight to a small PR against
   `master`: `--no-cursor` (10 lines, man page, `--help`). The description gives
   the "cursor is drawn separately, later" use case and points out that `grim`
   (without `-c`) and wl-screenrec (`--no-cursor`) already do this.
2. In parallel — a comment/PR on soreau's `ext-copy-capture` branch: the same
   option as `options = no_cursor ? 0 : PAINT_CURSORS`. That way the flag
   survives the port whichever lands first. #347 doesn't touch argument parsing
   or `--help` (8 of the patch's 10 lines) — only the line where the flag is
   used changes.

   **Unverified:** whether the cursor session survives next to an
   ext-copy-capture capture without the cursor. To check: build soreau's branch
   with that line (`wf-recorder-ecc/`), run curlog + `swaymsg seat seat0 cursor
   set` as with grim. If it breaks there too, that's an argument for a wlroots
   bug (item 3).
3. Separately, report to wlroots/sway that wlr-screencopy's `overlay_cursor=1`
   breaks the ext-image-copy-capture cursor session — a bug, not a feature; the
   minimal reproducer is curlog + `grim -c` in a loop.
4. Once everything is merged, `wf-recorder-nc` is no longer needed and
   screenrec switches to the system wf-recorder as soon as Ubuntu's package
   catches up.

