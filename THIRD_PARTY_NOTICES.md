# Third-party notices / Avis relatifs aux composants tiers

[English](#english) · [Français](#français)

---

## English

This project (Excalicloud) is an independent project. It is **not affiliated with, endorsed by or sponsored by Excalidraw** or Excalidraw s.r.o. "Excalidraw" is a trademark of its owner and is used here only to describe compatibility: this project embeds the open source [`@excalidraw/excalidraw`](https://www.npmjs.com/package/@excalidraw/excalidraw) component.

The project's own code is released under the MIT License (see [`LICENSE`](LICENSE)). The Docker image and the Go binary also redistribute the third-party components listed below, each under its own license.

### Excalidraw

- Source: <https://github.com/excalidraw/excalidraw>, npm package `@excalidraw/excalidraw`
- License: MIT

```text
MIT License

Copyright (c) 2020 Excalidraw

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
```

### Tabler Icons

The icons in `frontend/src/icons.tsx` are taken from [Tabler Icons](https://tabler.io/icons).

- Source: <https://github.com/tabler/tabler-icons>
- License: MIT

```text
MIT License

Copyright (c) 2020-2026 Paweł Kuna

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
```

### Fonts

The `@excalidraw/excalidraw` package ships fonts that are copied into the build (`frontend/scripts/copy-fonts.mjs`) and served locally. They are redistributed unmodified, with the software, as their licenses allow; they may not be sold on their own.

| Font | License | Source |
|---|---|---|
| Excalifont | SIL Open Font License 1.1 | <https://plus.excalidraw.com/excalifont> |
| Virgil | SIL Open Font License 1.1 | <https://plus.excalidraw.com/virgil> |
| Cascadia Code | SIL Open Font License 1.1 | <https://github.com/microsoft/cascadia-code> |
| Assistant | SIL Open Font License 1.1 | <https://fonts.google.com/specimen/Assistant> |
| Nunito | SIL Open Font License 1.1 | <https://fonts.google.com/specimen/Nunito> |
| Lilita One | SIL Open Font License 1.1 | <https://fonts.google.com/specimen/Lilita+One> |
| Liberation Sans | SIL Open Font License 1.1 | <https://github.com/liberationfonts/liberation-fonts> |
| Xiaolai | SIL Open Font License 1.1 | <https://github.com/lxgw/kose-font> |
| Comic Shanns | MIT | <https://github.com/shannpersand/comic-shanns> |

Full text of the SIL Open Font License 1.1: <https://openfontlicense.org/open-font-license-official-text/>

### Other bundled npm dependencies

The frontend bundle includes `react`, `react-dom` (MIT, Copyright (c) Meta Platforms, Inc. and affiliates) and the dependencies of `@excalidraw/excalidraw` (installed from `frontend/package-lock.json`, production dependencies). All are under permissive licenses; each package's full license text is in its own directory under `node_modules/`. For `dompurify`, dual-licensed, the Apache-2.0 option is used.

<details>
<summary>Dependencies by license</summary>

| License | Count | Packages |
|---|---|---|
| MIT | 168 | `@antfu/install-pkg`, `@babel/runtime`, `@braintree/sanitize-url`, `@excalidraw/excalidraw`, `@excalidraw/laser-pointer`, `@excalidraw/markdown-to-text`, `@excalidraw/mermaid-to-excalidraw`, `@excalidraw/random-username`, `@floating-ui/core`, `@floating-ui/dom`, `@floating-ui/react-dom`, `@floating-ui/utils`, `@iconify/types`, `@iconify/utils`, `@mermaid-js/parser`, `@radix-ui/primitive`, `@radix-ui/react-arrow`, `@radix-ui/react-collection`, `@radix-ui/react-compose-refs`, `@radix-ui/react-context`, `@radix-ui/react-direction`, `@radix-ui/react-dismissable-layer`, `@radix-ui/react-focus-guards`, `@radix-ui/react-focus-scope`, `@radix-ui/react-id`, `@radix-ui/react-popover`, `@radix-ui/react-popper`, `@radix-ui/react-portal`, `@radix-ui/react-presence`, `@radix-ui/react-primitive`, `@radix-ui/react-roving-focus`, `@radix-ui/react-slot`, `@radix-ui/react-tabs`, `@radix-ui/react-use-callback-ref`, `@radix-ui/react-use-controllable-state`, `@radix-ui/react-use-escape-keydown`, `@radix-ui/react-use-layout-effect`, `@radix-ui/react-use-rect`, `@radix-ui/react-use-size`, `@radix-ui/rect`, `@types/d3`, `@types/d3-array`, `@types/d3-axis`, `@types/d3-brush`, `@types/d3-chord`, `@types/d3-color`, `@types/d3-contour`, `@types/d3-delaunay`, `@types/d3-dispatch`, `@types/d3-drag`, `@types/d3-dsv`, `@types/d3-ease`, `@types/d3-fetch`, `@types/d3-force`, `@types/d3-format`, `@types/d3-geo`, `@types/d3-hierarchy`, `@types/d3-interpolate`, `@types/d3-path`, `@types/d3-polygon`, `@types/d3-quadtree`, `@types/d3-random`, `@types/d3-scale`, `@types/d3-scale-chromatic`, `@types/d3-selection`, `@types/d3-shape`, `@types/d3-time`, `@types/d3-time-format`, `@types/d3-timer`, `@types/d3-transition`, `@types/d3-zoom`, `@types/geojson`, `@types/react`, `@types/react-dom`, `@types/trusted-types`, `@upsetjs/venn.js`, `aria-hidden`, `binary-extensions`, `braces`, `canvas-roundrect-polyfill`, `chevrotain-allstar`, `chokidar`, `clsx`, `commander`, `cose-base`, `cross-env`, `cross-spawn`, `csstype`, `cytoscape`, `cytoscape-cose-bilkent`, `cytoscape-fcose`, `dagre-d3-es`, `dayjs`, `detect-node-es`, `es-toolkit`, `es6-promise-pool`, `fastdom`, `fill-range`, `fuzzy`, `get-nonce`, `glur`, `hachure-fill`, `iconv-lite`, `image-blob-reduce`, `immutable`, `import-meta-resolve`, `is-binary-path`, `is-extglob`, `is-glob`, `is-number`, `jotai`, `jotai-scope`, `katex`, `khroma`, `langium`, `layout-base`, `lodash-es`, `lodash.debounce`, `lodash.throttle`, `marked`, `mermaid`, `multimath`, `nanoid`, `normalize-path`, `object-assign`, `open-color`, `package-manager-detector`, `path-data-parser`, `path-key`, `perfect-freehand`, `pica`, `picomatch`, `png-chunk-text`, `png-chunks-encode`, `png-chunks-extract`, `points-on-curve`, `points-on-path`, `react`, `react-dom`, `react-remove-scroll`, `react-remove-scroll-bar`, `react-style-singleton`, `readdirp`, `roughjs`, `safer-buffer`, `sass`, `scheduler`, `shebang-command`, `shebang-regex`, `sliced`, `strictdom`, `stylis`, `tinyexec`, `to-regex-range`, `ts-dedent`, `tunnel-rat`, `use-callback-ref`, `use-sidecar`, `use-sync-external-store`, `uuid`, `vscode-jsonrpc`, `vscode-languageserver`, `vscode-languageserver-protocol`, `vscode-languageserver-textdocument`, `vscode-languageserver-types`, `vscode-uri`, `webworkify`, `zustand` |
| ISC | 37 | `anymatch`, `d3`, `d3-array`, `d3-axis`, `d3-brush`, `d3-chord`, `d3-color`, `d3-contour`, `d3-delaunay`, `d3-dispatch`, `d3-drag`, `d3-dsv`, `d3-fetch`, `d3-force`, `d3-format`, `d3-geo`, `d3-hierarchy`, `d3-interpolate`, `d3-path`, `d3-polygon`, `d3-quadtree`, `d3-random`, `d3-scale`, `d3-scale-chromatic`, `d3-selection`, `d3-shape`, `d3-time`, `d3-time-format`, `d3-timer`, `d3-transition`, `d3-zoom`, `delaunator`, `glob-parent`, `inherits`, `internmap`, `isexe`, `which` |
| Apache-2.0 | 9 | `@chevrotain/cst-dts-gen`, `@chevrotain/gast`, `@chevrotain/regexp-to-ast`, `@chevrotain/types`, `@chevrotain/utils`, `browser-fs-access`, `chevrotain`, `crc-32`, `pwacompat` |
| BSD-3-Clause | 7 | `d3-array`, `d3-ease`, `d3-path`, `d3-sankey`, `d3-shape`, `rw`, `source-map-js` |
| 0BSD | 1 | `tslib` |
| Unlicense | 1 | `robust-predicates` |
| CC0-1.0 | 1 | `fractional-indexing` |
| MIT AND Zlib | 1 | `pako` |
| MPL-2.0 OR Apache-2.0 | 1 | `dompurify` |

</details>

### Go backend

The Go backend uses only the Go standard library (no third-party modules in `go.mod`). The Go toolchain and standard library are distributed under a BSD-3-Clause license: <https://go.dev/LICENSE>.

The final Docker image is based on `gcr.io/distroless/static-debian12` (Apache-2.0, <https://github.com/GoogleContainerTools/distroless>); the Debian packages it contains (CA certificates, tzdata, base files) keep their own licenses, listed in the image under `/usr/share/doc`.

---

## Français

Ce projet (Excalicloud) est un projet indépendant. Il **n'est ni affilié à Excalidraw ou Excalidraw s.r.o., ni approuvé ou soutenu par eux**. « Excalidraw » est une marque de son titulaire, citée ici uniquement pour décrire la compatibilité : le projet intègre le composant open source [`@excalidraw/excalidraw`](https://www.npmjs.com/package/@excalidraw/excalidraw).

Le code propre au projet est publié sous licence MIT (voir [`LICENSE`](LICENSE), traduction informative dans [`LICENSE.fr.md`](LICENSE.fr.md)). L'image Docker et le binaire Go redistribuent aussi les composants tiers suivants, chacun sous sa propre licence. Les textes de licence reproduits dans la section anglaise ci-dessus sont ceux qui font foi.

- **Excalidraw** (`@excalidraw/excalidraw`) : licence MIT, Copyright (c) 2020 Excalidraw, <https://github.com/excalidraw/excalidraw>.
- **Tabler Icons** (icônes de `frontend/src/icons.tsx`) : licence MIT, Copyright (c) 2020-2026 Paweł Kuna, <https://github.com/tabler/tabler-icons>.
- **Polices** fournies par le paquet Excalidraw, copiées au build (`frontend/scripts/copy-fonts.mjs`) et servies localement : Excalifont, Virgil, Cascadia Code, Assistant, Nunito, Lilita One, Liberation Sans et Xiaolai sous SIL Open Font License 1.1 ; Comic Shanns sous MIT. Elles sont redistribuées sans modification, avec le logiciel, comme leurs licences le permettent ; elles ne peuvent pas être vendues seules. Sources dans le tableau de la section anglaise.
- **Autres dépendances npm embarquées** : `react`, `react-dom` (MIT, Copyright (c) Meta Platforms, Inc. and affiliates) et les dépendances de `@excalidraw/excalidraw`, toutes sous licences permissives (MIT, ISC, Apache-2.0, BSD-3-Clause, 0BSD, Unlicense, CC0-1.0, Zlib ; option Apache-2.0 retenue pour `dompurify`). Liste détaillée dans la section anglaise ; le texte complet de chaque licence se trouve dans le répertoire du paquet sous `node_modules/`.
- **Backend Go** : bibliothèque standard uniquement (licence BSD-3-Clause, <https://go.dev/LICENSE>). Image finale basée sur `gcr.io/distroless/static-debian12` (Apache-2.0) ; les paquets Debian qu'elle contient conservent leurs licences, consultables dans l'image sous `/usr/share/doc`.
