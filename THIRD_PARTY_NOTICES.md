# Third-party components

The admin application bundles React and React DOM (MIT). Frontend versions and integrity hashes are recorded in `internal/control/adminweb/package-lock.json`; Vite preserves their bundled license comments. The build uses Vite, TypeScript and related development tools under their respective licenses. IP geolocation uses `github.com/oschwald/maxminddb-golang` (ISC). The control service downloads [DB-IP City Lite](https://db-ip.com/db/download/ip-to-city-lite) by default under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/); pages displaying its results link to DB-IP. The database is cached at runtime, not bundled in release archives. Operator-supplied databases retain their own license and attribution requirements.

NodeLane Room links Nebula (MIT), based on official `github.com/slackhq/nebula v1.11.1` with an authorized handshake cache race fix from `github.com/Wy2926/nebula`, commit `d929786cba7f`. `go.mod` pins the exact replacement version; the fork retains the upstream module path, dependencies and license. See the fork's `NODELANE_PATCH.md` for the change and its limits. NodeLane does not add TURN. Windows bundles the unchanged upstream Wintun binary from that resolved module, with its license files retained. Wintun has its own distribution terms; do not replace it with an unverified download.

`go.mod` and `go.sum` record dependencies and checksums. Release archives include `licenses/modules.txt` and available top-level LICENSE/COPYING/NOTICE/PATENTS files from every resolved dependency, plus the Wintun distribution files. Build with `go mod verify`. Review all included notices before redistributing a commercial installer. NodeLane executable code signing is a separate release step.

## React and React DOM license

MIT License

Copyright (c) Meta Platforms, Inc. and affiliates.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## 桌面图标

客户端使用 `@phosphor-icons/react 2.1.10`，MIT License，Copyright Phosphor Icons。许可随 npm 包与桌面依赖许可清单提供。开发预览中的游戏图片来自对应 Steam 商店的公开素材，版权归各游戏权利人；不随正式客户端打包。
