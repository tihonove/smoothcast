# Roadmap

In order of importance:

1. **Patch and send upstream** (see [upstream.md](upstream.md)).
2. **Zoom on screen changes.** The H.264 skip-block map from
   `ffmpeg -debug mb_type -threads 1` (checked — works): after a click, look at
   where the screen changed and fit the zoom box so it holds both the click and
   the change; if almost the whole screen changed, don't zoom. The same signal
   catches touchpad taps: the cursor stood still and the screen changed next to
   it — a click.
3. **Cursor**: auto-hide when idle (Capptivo has it, not ported), cursor shape
   (hand over links, I-beam) — the cursor session provides the image; a
   "squeeze" on click.
4. **Frame quality**: crop in yuv444 so a slow camera doesn't jitter by 1–2 px;
   pick a default `-cursor-size`.
5. **Convenience**: defaults in a config file rather than flags; re-rendering
   right from Video Trimmer or via a separate `screenrec --rebuild`.
6. **A window rather than a screen area**: needs ext-copy-capture toplevel
   capture — comes with PR #347.
