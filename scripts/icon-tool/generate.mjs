// Regenerates every Apnoro icon asset from the launcher artwork assets/icon/apnoro_launcher.png.
//   cd scripts/icon-tool && npm install && node generate.mjs
//
// 1. Clean: drop the faint glow (alpha < 8) that pads the artwork, make the
//    body fully opaque (it ships at alpha 253), crop tight to the artwork.
//    The result is saved as assets/icon/apnoro_launcher_clean.png (master).
// 2. Outputs (assets/icons/): app-<size>.png, app.ico, app.icns, tray-*.ico,
//    installer-wizard-*.bmp; copies app-1024.png -> build/appicon.png and
//    app.ico -> build/windows/icon.ico (Wails), and the GUI/site logos.
import sharp from 'sharp';
import { Resvg } from '@resvg/resvg-js';
import { PNG } from 'pngjs';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..', '..');
const srcDir = path.join(root, 'assets', 'icon');
const out = path.join(root, 'assets', 'icons');
fs.mkdirSync(out, { recursive: true });

// ---------- 1. clean + crop the artwork
const GLOW_CUTOFF = 8; // alpha below this is the soft halo: removed
const SOLID_FROM = 250; // alpha at/above this becomes fully opaque

const src = PNG.sync.read(fs.readFileSync(path.join(srcDir, 'apnoro_launcher.png')));
let x0 = src.width, y0 = src.height, x1 = -1, y1 = -1;
for (let y = 0; y < src.height; y++) {
  for (let x = 0; x < src.width; x++) {
    const i = (y * src.width + x) * 4;
    const a = src.data[i + 3];
    if (a < GLOW_CUTOFF) {
      src.data[i] = src.data[i + 1] = src.data[i + 2] = src.data[i + 3] = 0;
      continue;
    }
    if (a >= SOLID_FROM) src.data[i + 3] = 255;
    if (x < x0) x0 = x;
    if (x > x1) x1 = x;
    if (y < y0) y0 = y;
    if (y > y1) y1 = y;
  }
}
const cropW = x1 - x0 + 1, cropH = y1 - y0 + 1;
const art = await sharp(PNG.sync.write(src))
  .extract({ left: x0, top: y0, width: cropW, height: cropH })
  .png()
  .toBuffer();
console.log(`artwork ${cropW}x${cropH} at (${x0},${y0}) of ${src.width}x${src.height}`);

// Square icon: artwork fitted inside a small, even margin (fraction of the side).
// Tiny sizes get less margin so the mark stays as large as possible.
const marginFor = (size) => (size <= 24 ? 0 : size <= 48 ? 0.02 : 0.04);

async function iconRGBA(size, artBuf = art) {
  const m = Math.round(size * marginFor(size));
  const inner = size - 2 * m;
  const { data } = await sharp(artBuf)
    .resize(inner, inner, { fit: 'contain', background: { r: 0, g: 0, b: 0, alpha: 0 }, kernel: 'lanczos3' })
    .extend({ top: m, bottom: m, left: m, right: m, background: { r: 0, g: 0, b: 0, alpha: 0 } })
    .ensureAlpha()
    .raw()
    .toBuffer({ resolveWithObject: true });
  return { rgba: data, width: size, height: size };
}
const toPng = ({ rgba, width, height }) =>
  sharp(rgba, { raw: { width, height, channels: 4 } }).png({ compressionLevel: 9 }).toBuffer();

// Master: tight, square, 1024 with the standard margin.
const master = await iconRGBA(1024);
fs.writeFileSync(path.join(srcDir, 'apnoro_launcher_clean.png'), await toPng(master));

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

// ---------- 2. app icons
const pngBySize = {};
const icoImages = [];
for (const s of [16, 24, 32, 48, 64, 128, 256, 512, 1024]) {
  const img = await iconRGBA(s);
  img.png = await toPng(img);
  pngBySize[s] = img.png;
  fs.writeFileSync(path.join(out, `app-${s}.png`), img.png);
  if (s <= 256) icoImages.push(img);
}
writeIco(path.join(out, 'app.ico'), icoImages);
writeIcns(path.join(out, 'app.icns'), pngBySize);

// ---------- tray icons: the mark plus a status dot (bottom-right, dark ring for contrast)
const dots = { running: '#3FB57A', stopped: '#8A8E97', error: '#E8622C' };
for (const [state, color] of Object.entries(dots)) {
  const imgs = [];
  for (const s of [16, 20, 24, 32]) {
    const base = await iconRGBA(s);
    const r = s * 0.22, cx = s - r - 0.5, cy = s - r - 0.5;
    const dot = Buffer.from(
      `<svg xmlns="http://www.w3.org/2000/svg" width="${s}" height="${s}"><circle cx="${cx}" cy="${cy}" r="${r}" fill="${color}" stroke="#16181D" stroke-width="${Math.max(1, s / 16)}"/></svg>`,
    );
    const { data } = await sharp(base.rgba, { raw: { width: s, height: s, channels: 4 } })
      .composite([{ input: dot }])
      .raw()
      .toBuffer({ resolveWithObject: true });
    const img = { rgba: data, width: s, height: s };
    imgs.push(img);
    if (s === 32) fs.writeFileSync(path.join(out, `tray-${state}-32.png`), await toPng(img));
  }
  writeIco(path.join(out, `tray-${state}.ico`), imgs);
}

// ---------- Inno Setup wizard images (100/125/150/175/200 % DPI)
function svgRGBA(svgText, width, height) {
  const r = new Resvg(svgText, {
    fitTo: { mode: 'width', value: width },
    font: { loadSystemFonts: true, defaultFontFamily: 'Segoe UI' },
    shapeRendering: 2,
  });
  const img = PNG.sync.read(r.render().asPng());
  return { rgba: img.data, width: img.width, height: img.height };
}
async function overlay(base, iconSize, left, top) {
  const icon = await iconRGBA(iconSize);
  const { data } = await sharp(base.rgba, { raw: { width: base.width, height: base.height, channels: 4 } })
    .composite([{ input: icon.rgba, raw: { width: iconSize, height: iconSize, channels: 4 }, left, top }])
    .raw()
    .toBuffer({ resolveWithObject: true });
  return { rgba: data, width: base.width, height: base.height };
}
const largePanel = (w, h) => `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 164 314" width="${w}" height="${h}" preserveAspectRatio="none">
  <rect width="164" height="314" fill="#16181D"/>
  <text x="82" y="206" text-anchor="middle" font-family="Segoe UI" font-weight="700" font-size="26" letter-spacing="2" fill="#F5F3EF">Apnoro</text>
  <text x="82" y="226" text-anchor="middle" font-family="Segoe UI" font-size="9.5" fill="#9A9DA5">Apache · MySQL · PHP</text>
</svg>`;
const smallPanel = (w, h) => `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}"><rect width="${w}" height="${h}" fill="#ffffff"/></svg>`;
for (const k of [1, 1.25, 1.5, 1.75, 2]) {
  const pct = Math.round(k * 100);
  const lw = Math.round(164 * k), lh = Math.round(314 * k);
  const ls = Math.round(96 * k);
  const large = await overlay(svgRGBA(largePanel(lw, lh), lw, lh), ls, Math.round((lw - ls) / 2), Math.round(76 * k));
  writeBmp24(path.join(out, `installer-wizard-large-${pct}.bmp`), large, [0x16, 0x18, 0x1d]);
  const sw = Math.round(55 * k), sh = Math.round(58 * k), ss = Math.round(52 * k);
  const small = await overlay(svgRGBA(smallPanel(sw, sh), sw, sh), ss, Math.round((sw - ss) / 2), Math.round((sh - ss) / 2));
  writeBmp24(path.join(out, `installer-wizard-small-${pct}.bmp`), small, [255, 255, 255]);
}
// Unsuffixed 100% copies for tools that want a single file.
fs.copyFileSync(path.join(out, 'installer-wizard-large-100.bmp'), path.join(out, 'installer-wizard-large.bmp'));
fs.copyFileSync(path.join(out, 'installer-wizard-small-100.bmp'), path.join(out, 'installer-wizard-small.bmp'));

// ---------- consumers
fs.mkdirSync(path.join(root, 'build', 'windows'), { recursive: true });
fs.copyFileSync(path.join(out, 'app-1024.png'), path.join(root, 'build', 'appicon.png'));
fs.copyFileSync(path.join(out, 'app.ico'), path.join(root, 'build', 'windows', 'icon.ico'));
fs.mkdirSync(path.join(root, 'frontend', 'src', 'assets'), { recursive: true });
fs.copyFileSync(path.join(out, 'app-64.png'), path.join(root, 'frontend', 'src', 'assets', 'logo.png'));
const site = path.join(root, 'website');
fs.mkdirSync(path.join(site, 'assets'), { recursive: true });
fs.copyFileSync(path.join(out, 'app-256.png'), path.join(site, 'assets', 'logo-256.png'));
fs.copyFileSync(path.join(out, 'app-64.png'), path.join(site, 'assets', 'logo-64.png'));
fs.copyFileSync(path.join(out, 'app.ico'), path.join(site, 'favicon.ico'));

console.log('icons written to', out);
