"""Generate the LAN Share app icon using only the stdlib.

A projector lens on a dark squircle throws a warm beam onto a picture: the
same amber lamp that lights in the app's window while the share is live.
Drawn once at 1024 px with signed-distance antialiasing, then halved for
the smaller sizes.
"""
import array
import binascii
import math
import pathlib
import struct
import zlib

DEST = pathlib.Path(__file__).resolve().parents[1] / "resources"
S = 1024

# Apple's macOS icon grid: an 824 px body centered on the 1024 canvas, its
# corners curving continuously (a superellipse blend rather than an arc).
CX = CY = 512.0
HALF = 412.0
CORNER, CORNER_EXP = 270.0, 3.0

LENS = (312.0, 704.0)
SCREEN = (478.0, 262.0, 846.0, 502.0)  # left, top, right, bottom
SCREEN_RADIUS = 14.0


def clamp(v, lo=0.0, hi=1.0):
    return lo if v < lo else hi if v > hi else v


def smooth(e0, e1, v):
    t = clamp((v - e0) / (e1 - e0))
    return t * t * (3 - 2 * t)


def mix(a, b, t):
    return tuple(x + (y - x) * t for x, y in zip(a, b))


def hexc(s):
    return tuple(int(s[i:i + 2], 16) / 255 for i in (1, 3, 5))


def body_sdf(x, y):
    """Signed distance to the body, negative inside."""
    qx = abs(x - CX) - (HALF - CORNER)
    qy = abs(y - CY) - (HALF - CORNER)
    ox, oy = max(qx, 0.0), max(qy, 0.0)
    return (ox ** CORNER_EXP + oy ** CORNER_EXP) ** (1 / CORNER_EXP) + min(max(qx, qy), 0.0) - CORNER


def rect_sdf(x, y, rect, radius):
    l, t, r, b = rect
    hw, hh = (r - l) / 2, (b - t) / 2
    qx = abs(x - (l + r) / 2) - hw + radius
    qy = abs(y - (t + b) / 2) - hh + radius
    return math.hypot(max(qx, 0.0), max(qy, 0.0)) + min(max(qx, qy), 0.0) - radius


def triangle_sdf(p, a, b, c):
    """Exact signed distance to a triangle (Inigo Quilez's sdTriangle)."""
    def sub(u, v):
        return (u[0] - v[0], u[1] - v[1])

    def dot(u, v):
        return u[0] * v[0] + u[1] * v[1]

    e0, e1, e2 = sub(b, a), sub(c, b), sub(a, c)
    v0, v1, v2 = sub(p, a), sub(p, b), sub(p, c)
    best, sign = float("inf"), 0.0
    s = 1.0 if e0[0] * e2[1] - e0[1] * e2[0] > 0 else -1.0
    for e, v in ((e0, v0), (e1, v1), (e2, v2)):
        h = clamp(dot(v, e) / dot(e, e))
        q = (v[0] - e[0] * h, v[1] - e[1] * h)
        best = min(best, dot(q, q))
        if s * (v[0] * e[1] - v[1] * e[0]) < 0:
            sign = 1.0
    return math.sqrt(best) * (1.0 if sign else -1.0)


BODY_TOP, BODY_BOTTOM = hexc("#3a322b"), hexc("#151210")
BEAM = hexc("#ffcf7d")
SCREEN_TOP, SCREEN_BOTTOM = hexc("#fff1d2"), hexc("#f7bd62")
SUN = hexc("#f19a34")
HILL_FAR, HILL_NEAR = hexc("#eba04a"), hexc("#d27e27")
HOUSING_TOP, HOUSING_BOTTOM = hexc("#544a40"), hexc("#2a241f")
GLASS_CORE, GLASS_EDGE = hexc("#fff4da"), hexc("#ee9a35")
GAP = hexc("#120f0d")

# The beam spreads from the lens to the screen's far corners.
BEAM_TRI = (LENS, (SCREEN[0] + 10, SCREEN[1] + 6), (SCREEN[2] - 6, SCREEN[3] - 10))
HILL_FAR_TRI = ((548.0, 540.0), (668.0, 384.0), (808.0, 540.0))
HILL_NEAR_TRI = ((660.0, 540.0), (762.0, 420.0), (880.0, 540.0))
SUN_AT, SUN_RADIUS = (566.0, 334.0), 30.0


def over(dst, color, alpha):
    """Composite a straight color with alpha over a premultiplied pixel."""
    if alpha <= 0:
        return dst
    r, g, b, a = dst
    k = 1 - alpha
    return (color[0] * alpha + r * k, color[1] * alpha + g * k, color[2] * alpha + b * k, alpha + a * k)


def pixel(x, y):
    out = (0.0, 0.0, 0.0, 0.0)
    d = body_sdf(x, y)
    out = over(out, (0, 0, 0), 0.30 * (1 - smooth(-24, 28, body_sdf(x, y - 12))))
    body = clamp(0.5 - d)
    if body <= 0:
        return out
    col = mix(BODY_TOP, BODY_BOTTOM, clamp((y - (CY - HALF)) / (2 * HALF)))
    lx, ly = x - LENS[0], y - LENS[1]
    dist = math.hypot(lx, ly)
    sd = rect_sdf(x, y, SCREEN, SCREEN_RADIUS)
    # Light spilling around the lens and off the screen.
    col = mix(col, BEAM, 0.22 * math.exp(-(dist / 190) ** 2))
    col = mix(col, BEAM, 0.30 * math.exp(-(max(sd, 0) / 70) ** 2))
    # The beam, brightest at the lens.
    beam = 1 - smooth(-2, 2, triangle_sdf((x, y), *BEAM_TRI))
    if beam > 0:
        reach = clamp(dist / 520)
        col = mix(col, BEAM, beam * (0.50 - 0.28 * reach))
    # Rim light along the top edge of the body.
    if d > -3 and y < CY:
        col = mix(col, (1, 1, 1), 0.10 * (1 - smooth(-3, 0, -d)) * (CY - y) / HALF)
    # The picture on the screen: a sun over two hills.
    screen = clamp(0.5 - sd)
    if screen > 0:
        g = mix(SCREEN_TOP, SCREEN_BOTTOM, clamp((y - SCREEN[1]) / (SCREEN[3] - SCREEN[1])))
        g = mix(g, SUN, clamp(SUN_RADIUS + 0.5 - math.hypot(x - SUN_AT[0], y - SUN_AT[1])))
        g = mix(g, HILL_FAR, clamp(0.5 - (triangle_sdf((x, y), *HILL_FAR_TRI) - 6)))
        g = mix(g, HILL_NEAR, clamp(0.5 - (triangle_sdf((x, y), *HILL_NEAR_TRI) - 6)))
        col = mix(col, g, screen)
    # The lens: housing, a dark gap, then the lit glass.
    housing = clamp(96.5 - dist)
    if housing > 0:
        col = mix(col, mix(HOUSING_TOP, HOUSING_BOTTOM, clamp((ly + 96) / 192)), housing)
        col = mix(col, (1, 1, 1), 0.14 * clamp(1 - abs(dist - 94) / 2) * clamp(-ly / 96))
    gap = clamp(72.5 - dist)
    if gap > 0:
        col = mix(col, GAP, gap)
    glass = clamp(62.5 - dist)
    if glass > 0:
        gd = math.hypot(lx + 14, ly + 14) / 76
        col = mix(col, mix(GLASS_CORE, GLASS_EDGE, smooth(0.05, 1.0, gd)), glass)
    return over(out, col, body)


def render():
    buf = array.array("f", bytes(S * S * 16))
    i = 0
    for py in range(S):
        y = py + 0.5
        for px in range(S):
            buf[i:i + 4] = array.array("f", pixel(px + 0.5, y))
            i += 4
    return buf, S


def halve(img):
    buf, size = img
    half = size // 2
    out = array.array("f", bytes(half * half * 16))
    for y in range(half):
        r0, r1 = (2 * y) * size * 4, (2 * y + 1) * size * 4
        for x in range(half):
            o = (y * half + x) * 4
            a, b = r0 + 8 * x, r1 + 8 * x
            for c in range(4):
                out[o + c] = (buf[a + c] + buf[a + 4 + c] + buf[b + c] + buf[b + 4 + c]) / 4
    return out, half


def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", binascii.crc32(kind + data) & 0xFFFFFFFF)


def png(img):
    buf, size = img
    rows = bytearray()
    for y in range(size):
        rows.append(0)
        for x in range(size):
            r, g, b, a = buf[(y * size + x) * 4:(y * size + x) * 4 + 4]
            if a > 0:
                r, g, b = r / a, g / a, b / a
            rows.extend(int(clamp(v) * 255 + 0.5) for v in (r, g, b, a))
    header = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(bytes(rows), 9)) + chunk(b"IEND", b"")


if __name__ == "__main__":
    images = {}
    img = render()
    while img[1] >= 16:
        images[img[1]] = img
        img = halve(img)
    entries = bytearray()
    for size, kind in [(16, b"icp4"), (32, b"icp5"), (64, b"icp6"), (128, b"ic07"), (256, b"ic08"), (512, b"ic09"), (1024, b"ic10")]:
        data = png(images[size])
        entries.extend(kind + struct.pack(">I", len(data) + 8) + data)
        if size == 1024:
            (DEST / "AppIcon.png").write_bytes(data)
    (DEST / "AppIcon.icns").write_bytes(b"icns" + struct.pack(">I", len(entries) + 8) + entries)
