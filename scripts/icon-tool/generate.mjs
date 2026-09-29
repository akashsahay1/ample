// Regenerates every AMPLS icon asset from the SVG sources in assets/icon/.
//   cd scripts/icon-tool && npm install && node generate.mjs
// Outputs (assets/icons/): app-<size>.png, app.ico, app.icns, tray-*.ico, installer-*.bmp
// and copies app-1024.png -> build/appicon.png, app.ico -> build/windows/icon.ico (Wails).
import { Resvg } from '@resvg/resvg-js';
import { PNG } from 'pngjs';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..', '..');
const src = path.join(root, 'assets', 'icon');
const out = path.join(root, 'assets', 'icons');
fs.mkdirSync(out, { recursive: true });

const svg = (name) => fs.readFileSync(path.join(src, name), 'utf8');
const FULL = svg('ampls.svg');
const S32 = svg('ampls-32.svg');
const S16 = svg('ampls-16.svg');

// Render an SVG string at width x height; returns { png: Buffer, rgba: Buffer (straight alpha) }.
function render(svgText, width, height = width) {
  const r = new Resvg(svgText, {
    fitTo: { mode: 'width', value: width },
    font: { loadSystemFonts: true, defaultFontFamily: 'Segoe UI' },
    shapeRendering: 2,
  });
  const png = r.render().asPng();
  const img = PNG.sync.read(png);
  if (img.width !== width || img.height !== height) {
    throw new Error(`render size ${img.width}x${img.height} != ${width}x${height}`);
  }
  return { png, rgba: img.data, width, height };
}

// App icon variant per size: simplified drawings for tiny sizes (per docs/design/Icon.dc.html).
const appSvgFor = (size) => (size <= 16 ? S16 : size <= 32 ? S32 : FULL);

// ---------- ICO (DIB entries < 256 for max compatibility, PNG entry for 256)
function dibEntry({ rgba, width, height }) {
  const header = Buffer.alloc(40);
  header.writeUInt32LE(40, 0);
  header.writeInt32LE(width, 4);
  header.writeInt32LE(height * 2, 8); // XOR + AND masks
  header.writeUInt16LE(1, 12);
  header.writeUInt16LE(32, 14);
  const xor = Buffer.alloc(width * height * 4);
  for (let y = 0; y < height; y++) {
    const srcRow = (height - 1 - y) * width * 4; // bottom-up
    for (let x = 0; x < width; x++) {
      const s = srcRow + x * 4, d = (y * width + x) * 4;
      xor[d] = rgba[s + 2]; xor[d + 1] = rgba[s + 1]; xor[d + 2] = rgba[s]; xor[d + 3] = rgba[s + 3];
    }
  }
  const maskStride = Math.ceil(width / 32) * 4;
  const and = Buffer.alloc(maskStride * height); // all 0 = use alpha
  return Buffer.concat([header, xor, and]);
}

function writeIco(file, images) {
  const entries = images.map((img) => (img.width >= 256 ? img.png : dibEntry(img)));
  const dir = Buffer.alloc(6 + 16 * images.length);
  dir.writeUInt16LE(0, 0); dir.writeUInt16LE(1, 2); dir.writeUInt16LE(images.length, 4);
  let offset = dir.length;
  images.forEach((img, i) => {
    const e = 6 + 16 * i;
    dir.writeUInt8(img.width >= 256 ? 0 : img.width, e);
    dir.writeUInt8(img.height >= 256 ? 0 : img.height, e + 1);
    dir.writeUInt8(0, e + 2); dir.writeUInt8(0, e + 3);
    dir.writeUInt16LE(1, e + 4); dir.writeUInt16LE(32, e + 6);
    dir.writeUInt32LE(entries[i].length, e + 8);
    dir.writeUInt32LE(offset, e + 12);
    offset += entries[i].length;
  });
  fs.writeFileSync(file, Buffer.concat([dir, ...entries]));
}

// ---------- ICNS (PNG entries)
function writeIcns(file, pngBySize) {
  const types = [['icp4', 16], ['icp5', 32], ['icp6', 64], ['ic07', 128], ['ic08', 256], ['ic09', 512], ['ic10', 1024],
    ['ic11', 32], ['ic12', 64], ['ic13', 256], ['ic14', 512]];
  const chunks = types.map(([t, s]) => {
    const data = pngBySize[s];
    const h = Buffer.alloc(8); h.write(t, 0, 'ascii'); h.writeUInt32BE(8 + data.length, 4);
    return Buffer.concat([h, data]);
  });
  const body = Buffer.concat(chunks);
  const h = Buffer.alloc(8); h.write('icns', 0, 'ascii'); h.writeUInt32BE(8 + body.length, 4);
  fs.writeFileSync(file, Buffer.concat([h, body]));
}

// ---------- 24-bit BMP (Inno Setup wizard images), alpha flattened onto `bg`
function writeBmp24(file, { rgba, width, height }, bg = [255, 255, 255]) {
  const stride = Math.ceil((width * 3) / 4) * 4;
  const size = 54 + stride * height;
  const b = Buffer.alloc(size);
  b.write('BM', 0, 'ascii'); b.writeUInt32LE(size, 2); b.writeUInt32LE(54, 10);
  b.writeUInt32LE(40, 14); b.writeInt32LE(width, 18); b.writeInt32LE(height, 22);
  b.writeUInt16LE(1, 26); b.writeUInt16LE(24, 28); b.writeUInt32LE(stride * height, 34);
  b.writeInt32LE(3780, 38); b.writeInt32LE(3780, 42);
  for (let y = 0; y < height; y++) {
    const row = 54 + (height - 1 - y) * stride;
    for (let x = 0; x < width; x++) {
      const s = (y * width + x) * 4, a = rgba[s + 3] / 255;
      const px = [0, 1, 2].map((c) => Math.round(rgba[s + c] * a + bg[c] * (1 - a)));
      b[row + x * 3] = px[2]; b[row + x * 3 + 1] = px[1]; b[row + x * 3 + 2] = px[0];
    }
  }
  fs.writeFileSync(file, b);
}

// ---------- app PNGs
const pngBySize = {};
for (const s of [16, 24, 32, 48, 64, 128, 256, 512, 1024]) {
  const img = render(appSvgFor(s), s);
  pngBySize[s] = img.png;
  fs.writeFileSync(path.join(out, `app-${s}.png`), img.png);
}
writeIco(path.join(out, 'app.ico'), [16, 24, 32, 48, 64, 128, 256].map((s) => render(appSvgFor(s), s)));
writeIcns(path.join(out, 'app.icns'), pngBySize);

// ---------- tray icons
for (const state of ['running', 'stopped', 'error']) {
  const t = svg(`tray-${state}.svg`);
  writeIco(path.join(out, `tray-${state}.ico`), [16, 20, 24, 32].map((s) => render(t, s)));
  fs.writeFileSync(path.join(out, `tray-${state}-32.png`), render(t, 32).png);
}

// ---------- Inno Setup wizard images (100/125/150/175/200 % DPI)
const scales = [1, 1.25, 1.5, 1.75, 2];
const innerFull = FULL.replace(/^<svg[^>]*>/, '').replace(/<\/svg>\s*$/, '');
function wizardLargeSvg(w, h) {
  // Designed on a 164x314 canvas, scaled via viewBox.
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 164 314" width="${w}" height="${h}" preserveAspectRatio="none">
  <rect width="164" height="314" fill="#16181D"/>
  <svg x="42" y="92" width="80" height="80" viewBox="0 0 256 256">${innerFull.replace('fill="#16181D"', 'fill="#22252D"')}</svg>
  <text x="82" y="206" text-anchor="middle" font-family="Segoe UI" font-weight="700" font-size="26" letter-spacing="2" fill="#F5F3EF">AMPLS</text>
  <text x="82" y="226" text-anchor="middle" font-family="Segoe UI" font-size="9.5" fill="#9A9DA5">Apache · MySQL · PHP</text>
  <rect x="58" y="282" width="12" height="4" rx="2" fill="#E8622C"/>
  <rect x="76" y="282" width="12" height="4" rx="2" fill="#9AA3FF"/>
  <rect x="94" y="282" width="12" height="4" rx="2" fill="#3FA9C9"/>
</svg>`;
}
function wizardSmallSvg(w, h) {
  // 55x58 canvas: the app icon, centered, on the white wizard header.
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 55 58" width="${w}" height="${h}">
  <svg x="1.5" y="3" width="52" height="52" viewBox="0 0 256 256">${innerFull}</svg>
</svg>`;
}
const largeFiles = [], smallFiles = [];
for (const k of scales) {
  const pct = Math.round(k * 100);
  const lw = Math.round(164 * k), lh = Math.round(314 * k);
  const sw = Math.round(55 * k), sh = Math.round(58 * k);
  const lf = `installer-wizard-large-${pct}.bmp`, sf = `installer-wizard-small-${pct}.bmp`;
  writeBmp24(path.join(out, lf), render(wizardLargeSvg(lw, lh), lw, lh), [0x16, 0x18, 0x1d]);
  writeBmp24(path.join(out, sf), render(wizardSmallSvg(sw, sh), sw, sh), [255, 255, 255]);
  largeFiles.push(lf); smallFiles.push(sf);
}
// Unsuffixed 100% copies for tools that want a single file.
fs.copyFileSync(path.join(out, 'installer-wizard-large-100.bmp'), path.join(out, 'installer-wizard-large.bmp'));
fs.copyFileSync(path.join(out, 'installer-wizard-small-100.bmp'), path.join(out, 'installer-wizard-small.bmp'));

// ---------- Wails build inputs
fs.mkdirSync(path.join(root, 'build', 'windows'), { recursive: true });
fs.copyFileSync(path.join(out, 'app-1024.png'), path.join(root, 'build', 'appicon.png'));
fs.copyFileSync(path.join(out, 'app.ico'), path.join(root, 'build', 'windows', 'icon.ico'));

console.log('icons written to', out);
