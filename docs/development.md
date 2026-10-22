# Development

## Building

```sh
make            # static binary ./smoothcast (CGO_ENABLED=0)
make install    # into ~/.local/bin, override with PREFIX=
```

Needs Go; at runtime only `ffmpeg` / `ffprobe`.

## Releasing

Packages for Ubuntu 24.04 and 26.04, amd64 and arm64, are published to the apt
repository [tihonove/apt](https://github.com/tihonove/apt): `smoothcast`,
`curlog`, `wf-recorder-nc`.

To release a version, push a tag `vX.Y.Z` in the tool's repository. CI
(`.github/workflows/release.yml`) builds the `.deb` files (`debian/build-deb`)
and attaches them to a GitHub Release; the apt repository picks them up by
itself.
