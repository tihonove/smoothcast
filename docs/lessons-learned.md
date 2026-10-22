# Lessons learned

Things found along the way on sway 1.11 / wlroots.

- Capturing with the cursor baked in (wlr-screencopy with `overlay_cursor=1`:
  wf-recorder, `grim -c`) kills sway 1.11's cursor session: `leave` right away,
  no more positions. Hence the fork: with no cursor in the frame wl-castlog gets
  every movement.
- Without the cursor in the frame its movement produces no damage, so
  wf-recorder gets no frames on a static screen. Hence `-D`; the cost is about
  one extra core at 3440×1440.
- wl-castlog has to open `get_capture_session`, otherwise the cursor source sends
  `leave` right away, even with no recording.
- Sync without a patch: wf-recorder prints `pts_time: 0` to stderr on the first
  frame, and screenrec stamps the lines with `$EPOCHREALTIME`. The fallback is
  wl-castlog's stop minus the duration; the two differ by ~26 ms.
- `swaymsg seat seat0 cursor set X Y` (output-relative coordinates) works for
  testing wl-castlog without a real mouse.
- Touchpad taps are invisible in evdev — only a button press counts as a click.
- xdg-desktop-portal-wlr 0.8.1 on sway 1.11 delivers no frames, fixed in 0.8.4;
  Capptivo was tried and rolled back — not the Linux way.
