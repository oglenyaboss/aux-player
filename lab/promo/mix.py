#!/usr/bin/env python3
"""Mixes the film's audio from lab/cache/promo/timeline.json: every segment starts on
its downbeat at the exact frame of its cut, the sound design is synthesized here."""
import json
import subprocess
import sys

import numpy as np
import pyloudnorm as pyln
import soundfile as sf
from scipy.signal import butter, fftconvolve, sosfilt

SR = 48000
WAVS = "lab/cache/promo/wav"
rng = np.random.default_rng(7)


def db(x):
    return 10 ** (x / 20)


def filt(y, kind, f, order=4):
    return sosfilt(butter(order, f, btype=kind, fs=SR, output="sos"), y, axis=0)


def stereo(m):
    return np.stack([m, m], 1)


def place(mix, y, t, gain=1.0):
    i = int(round(t * SR))
    if i < 0:
        y, i = y[-i:], 0
    n = min(len(y), len(mix) - i)
    if n > 0:
        mix[i:i + n] += y[:n] * gain


def fade(y, a, b):
    y = y.copy()
    na, nb = int(a * SR), int(b * SR)
    if na:
        y[:na] *= np.linspace(0, 1, na)[:, None]
    if nb:
        y[-nb:] *= np.linspace(1, 0, nb)[:, None]
    return y


def sub_kick():
    n = int(0.42 * SR)
    t = np.arange(n) / SR
    f = 42 + 80 * np.exp(-t * 38)
    return stereo(np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-t * 9))


def needle():
    n = int(0.28 * SR)
    t = np.arange(n) / SR
    thud = np.sin(2 * np.pi * 70 * t) * np.exp(-t * 24)
    pop = filt(rng.standard_normal(n), "low", 3500, 2) * np.exp(-t * 90)
    return stereo(thud + 0.5 * pop)


def crackle(sec):
    n = int(sec * SR)
    y = filt(rng.standard_normal(n), "band", [300, 4000], 2) * 0.004
    for _ in range(int(sec * 9)):
        i = rng.integers(0, n - 500)
        w = rng.integers(40, 300)
        y[i:i + w] += rng.standard_normal(w) * np.hanning(w) * rng.uniform(0.02, 0.16)
    left = filt(y, "high", 500, 1)
    right = np.roll(left, 37)
    return np.stack([left, right], 1)


def riser(sec):
    n = int(sec * SR)
    t = np.linspace(0, 1, n)
    noise = rng.standard_normal(n)
    lo = filt(noise, "band", [400, 1600], 2)
    hi = filt(noise, "band", [2500, 9000], 2)
    return stereo((lo * (1 - t) + hi * t) * t ** 2.6)


def reverb_ir(sec, bright=4500):
    n = int(sec * SR)
    t = np.arange(n) / SR
    env = np.exp(-t * 6.9 / sec)
    ir = np.stack([rng.standard_normal(n), rng.standard_normal(n)], 1) * env[:, None]
    return filt(ir, "low", bright, 2) * 0.05


def verb(y, ir):
    return np.stack([fftconvolve(y[:, c], ir[:, c]) for c in (0, 1)], 1)


def load(key):
    y, sr = sf.read(f"{WAVS}/{key}.wav")
    assert sr == SR
    return y


def lufs_gain(y, target):
    return db(target - pyln.Meter(SR).integrated_loudness(y))


def main(timeline, out):
    tl = json.load(open(timeline))
    fps = 60
    end = tl["end"] / fps
    drop = tl["drop"] / fps
    last = tl["last"] / fps
    beat = tl["beat"]
    bar = 4 * beat
    mix = np.zeros((int((end + 0.5) * SR), 2))

    # bed: the bar before HAHAHA's first downbeat, looped, opening up into the drop
    first = tl["segments"][0]
    src = load(first["key"])
    i1 = int(first["in"] * SR)
    i0 = int((first["in"] - bar) * SR)
    loop = src[i0:i1] * db(8)
    bars = 7
    bed = np.concatenate([fade(loop, 0.01, 0.01)] * bars)
    n = len(bed)
    sweep = np.zeros_like(bed)
    steps = 40
    for s in range(steps):
        k = s / (steps - 1)
        a, b = s * n // steps, (s + 1) * n // steps
        cut = 250 * (9000 / 250) ** (k ** 1.5)
        sweep[a:b] = filt(bed[max(0, a - 4000):b], "low", cut, 2)[-(b - a):]
    gain = np.linspace(0.5, 0.6, n)[:, None]
    place(mix, sweep * gain, drop - bars * bar)

    # pulse under the preroll, doubling over the last bars
    for k in range(7, int(drop / beat)):
        at = k * beat
        if at >= drop - 0.2:
            break
        if k < 16 and k % 2:
            continue
        place(mix, sub_kick(), at, db(-13 + 2 * k / 28))
    place(mix, riser(2 * bar - beat), drop - 2 * bar, db(-22))
    place(mix, crackle(drop - 8.0), 8.0, db(-8))

    for ev in tl["sfx"]:
        place(mix, needle(), ev["t"], db(-4))

    # a held breath before the drop
    a, b = int((drop - 0.14) * SR), int(drop * SR)
    mix[a:b] *= np.linspace(1, 0.12, b - a)[:, None]

    # the music: every segment on its own downbeat, at one loudness
    ir = reverb_ir(1.6)
    for i, s in enumerate(tl["segments"]):
        y = load(s["key"])
        t = s["start"] / fps
        dur = s["frames"] / fps
        a = int(s["in"] * SR)
        seg = y[a:a + int(dur * SR) + 480]
        g = lufs_gain(seg, -13)
        tail_in = i + 1 < len(tl["segments"])
        place(mix, fade(seg, 0.002, 0.01), t, g)
        if tail_in and i > 0:
            throw = fade(seg[-int(0.5 * 60 / s["bpm"] * SR):], 0.005, 0.02)
            place(mix, verb(throw, ir), t + dur, g * db(-14))
        print(f'{s["key"]:16s} t={t:7.3f} in={s["in"]:7.3f} gain {20 * np.log10(g):+.1f} dB')

    # the ending: the downbeat that never came, thrown into a hall, the needle lifts
    final = tl["segments"][-1]
    y = load(final["key"])
    a = int((final["in"] + final["frames"] / fps) * SR)
    hit = fade(y[a:a + int(0.42 * SR)], 0.001, 0.2) * lufs_gain(y[a:a + SR], -13)
    place(mix, hit, last)
    big = reverb_ir(5.5, 3200)
    place(mix, verb(hit, big), last, db(-6))
    place(mix, sub_kick(), last, db(-5))
    place(mix, crackle(end - last), last, db(-13))
    place(mix, needle(), end - 1.1, db(-13))

    n = int(end * SR)
    mix = fade(mix[:n], 0.5, 0.35)
    meter = pyln.Meter(SR)
    played = mix[int(drop * SR):int(last * SR)]
    mix *= db(-14 - meter.integrated_loudness(played) + 1.0)
    tmp = out + ".raw.wav"
    sf.write(tmp, mix, SR, subtype="FLOAT")
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", tmp, "-af",
                    "alimiter=limit=0.8:attack=1:release=60:level=disabled", "-c:a", "pcm_s24le", out], check=True)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
