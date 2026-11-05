#!/usr/bin/env python3
"""Bakes the terminal glyph atlas for lab/promo: 24x48 alpha masks, 160x45 cells at 4K."""
import math
import os
import struct
import sys

from PIL import Image, ImageDraw, ImageFont

CW, CH, SS = 12, 24, 8
OUT = (24, 48)
FIRA = os.path.expanduser("~/.local/share/fonts/nerd-fonts/FiraCodeNerdFontMono-%s.ttf")
FALLBACK = ["/usr/share/fonts/noto/NotoSansSymbols-Regular.ttf", "/usr/share/fonts/noto/NotoSansSymbols2-Regular.ttf",
            "/usr/share/fonts/TTF/DejaVuSans.ttf"]

RANGES = [(0x20, 0x7E), (0xA0, 0xFF), (0x400, 0x45F), (0x2010, 0x2044), (0x2190, 0x2199),
          (0x2500, 0x259F), (0x25A0, 0x25FF), (0x2600, 0x26FF), (0x2700, 0x27BF), (0x2800, 0x28FF)]


def box(draw_fn):
    im = Image.new("L", (CW * SS, CH * SS), 0)
    draw_fn(ImageDraw.Draw(im), CW * SS, CH * SS)
    return im.resize(OUT, Image.BOX)


def rect(x0, y0, x1, y1):
    return box(lambda d, w, h: d.rectangle([x0 * w, y0 * h, x1 * w - 1, y1 * h - 1], fill=255))


def line(*segs, t=2):
    """segs are of 'l','r','u','d': arms from the cell centre."""
    def f(d, w, h):
        cx, cy, half = w / 2, h / 2, t * SS / 2
        for s in segs:
            x0, y0, x1, y1 = {"l": (0, cy - half, cx + half, cy + half), "r": (cx - half, cy - half, w, cy + half),
                              "u": (cx - half, 0, cx + half, cy + half), "d": (cx - half, cy - half, cx + half, h)}[s]
            d.rectangle([x0, y0, x1 - 1, y1 - 1], fill=255)
    return box(f)


def corner(sx, sy, t=2):
    """A rounded corner: one arm runs out sideways towards sx, the other vertically towards sy."""
    def f(d, w, h):
        cx, cy, r = w / 2, h / 2, w / 2
        ax, ay = cx + sx * r, cy + sy * r
        arc = [(ax - sx * r * math.cos(a), ay - sy * r * math.sin(a))
               for a in (math.pi / 2 * i / 64 for i in range(65))]
        d.line(arc, fill=255, width=t * SS, joint="curve")
        d.rectangle([cx - t * SS / 2, min(ay, h if sy > 0 else 0), cx + t * SS / 2 - 1, max(ay, h if sy > 0 else 0)], fill=255)
    return box(f)


def shade(a):
    return Image.new("L", OUT, int(255 * a))


def braille(cp):
    bits = cp - 0x2800
    dots = [(0, 0, 0), (0, 1, 1), (0, 2, 2), (1, 0, 3), (1, 1, 4), (1, 2, 5), (0, 3, 6), (1, 3, 7)]

    def f(d, w, h):
        r = w * 0.13
        for dx, dy, b in dots:
            if bits >> b & 1:
                x, y = (dx + 0.5) * w / 2, (dy + 0.5) * h / 4
                d.ellipse([x - r, y - r, x + r, y + r], fill=255)
    return box(f)


SPECIAL = {
    0x2580: rect(0, 0, 1, .5), 0x2584: rect(0, .5, 1, 1), 0x2588: rect(0, 0, 1, 1), 0x258C: rect(0, 0, .5, 1),
    0x2590: rect(.5, 0, 1, 1), 0x258E: rect(0, 0, .25, 1), 0x2594: rect(0, 0, 1, .125), 0x2581: rect(0, .875, 1, 1),
    0x2591: shade(.25), 0x2592: shade(.5), 0x2593: shade(.75),
    0x2500: line("l", "r"), 0x2502: line("u", "d"), 0x253C: line("l", "r", "u", "d"), 0x2501: line("l", "r", t=3),
    0x2503: line("u", "d", t=3), 0x253F: line("l", "r", t=3), 0x250C: line("r", "d"), 0x2510: line("l", "d"),
    0x2514: line("r", "u"), 0x2518: line("l", "u"), 0x251C: line("r", "u", "d"), 0x2524: line("l", "u", "d"),
    0x252C: line("l", "r", "d"), 0x2534: line("l", "r", "u"), 0x255F: line("r", "u", "d"), 0x2562: line("l", "u", "d"),
    0x256D: corner(1, 1), 0x256E: corner(-1, 1), 0x256F: corner(-1, -1), 0x2570: corner(1, -1),
}


def load(path, size):
    try:
        return ImageFont.truetype(path, size)
    except OSError:
        return None


def covers(path):
    from fontTools.ttLib import TTFont
    return set(TTFont(path, fontNumber=0).getBestCmap())


def glyph(cp, font, fallback):
    im = Image.new("L", (CW * SS, CH * SS), 0)
    d = ImageDraw.Draw(im)
    ch = chr(cp)
    if font is not None:
        d.text((0, 19 * SS), ch, fill=255, font=font, anchor="ls")
    else:
        l, t, r, b = fallback.getbbox(ch, anchor="ls")
        scale = min(1, CW * SS / max(1, r - l))
        tmp = Image.new("L", (max(1, r - l), max(1, b - t)), 0)
        ImageDraw.Draw(tmp).text((-l, -t), ch, fill=255, font=fallback, anchor="ls")
        w, h = max(1, int(tmp.width * scale)), max(1, int(tmp.height * scale))
        im.paste(tmp.resize((w, h), Image.LANCZOS), ((CW * SS - w) // 2, (CH * SS - h) // 2 - SS))
    return im.resize(OUT, Image.BOX)


def main(out):
    fonts = {False: (FIRA % "Medium"), True: (FIRA % "Bold")}
    cmap = covers(fonts[False])
    fb = [(p, load(p, 18 * SS)) for p in FALLBACK]
    fb = [(covers(p), f) for p, f in fb if f is not None]
    entries = []
    for bold, path in fonts.items():
        main_font = load(path, 20 * SS)
        for lo, hi in RANGES:
            for cp in range(lo, hi + 1):
                if cp in SPECIAL:
                    im = SPECIAL[cp]
                elif 0x2800 <= cp <= 0x28FF:
                    im = braille(cp)
                elif cp in cmap:
                    im = glyph(cp, main_font, None)
                else:
                    f = next((f for c, f in fb if cp in c), None)
                    if f is None:
                        continue
                    im = glyph(cp, None, f)
                entries.append(struct.pack("<IB", cp, int(bold)) + im.tobytes())
    with open(out, "wb") as w:
        w.write(struct.pack("<I", len(entries)))
        w.writelines(entries)
    print(len(entries), "glyphs ->", out)


if __name__ == "__main__":
    main(sys.argv[1])
