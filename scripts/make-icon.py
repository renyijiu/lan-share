"""Generate the original LAN Share folder/network icon using only the stdlib."""
import binascii
import math
import pathlib
import struct
import zlib

DEST = pathlib.Path(__file__).resolve().parents[1] / "resources"


def inside_round(x, y, left, top, right, bottom, radius):
    cx = max(left + radius, min(x, right - radius))
    cy = max(top + radius, min(y, bottom - radius))
    return (x - cx) ** 2 + (y - cy) ** 2 <= radius ** 2


def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", binascii.crc32(kind + data) & 0xFFFFFFFF)


def png(size):
    rows = bytearray()
    for py in range(size):
        rows.append(0)
        for px in range(size):
            x, y = (px + .5) / size, (py + .5) / size
            color = (0, 0, 0, 0)
            if inside_round(x, y, .08, .08, .92, .92, .19):
                color = (int(20 + y * 2), int(145 - y * 40), int(135 - y * 30), 255)
                folder = inside_round(x, y, .235, .37, .765, .695, .045)
                tab = inside_round(x, y, .235, .31, .46, .45, .03)
                if folder or tab:
                    color = (240, 254, 250, 255)
                distance = math.hypot(x - .5, y - .67)
                if y < .62 and .3 < x < .7 and (abs(distance - .145) < .015 or abs(distance - .225) < .015):
                    color = (24, 125, 115, 255)
                if math.hypot(x - .5, y - .625) < .026:
                    color = (24, 125, 115, 255)
            rows.extend(color)
    header = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(rows, 9)) + chunk(b"IEND", b"")


if __name__ == "__main__":
    entries = bytearray()
    for size, kind in [(16,b"icp4"),(32,b"icp5"),(64,b"icp6"),(128,b"ic07"),(256,b"ic08"),(512,b"ic09"),(1024,b"ic10")]:
        data = png(size)
        entries.extend(kind + struct.pack(">I", len(data) + 8) + data)
        if size == 1024:
            (DEST / "AppIcon.png").write_bytes(data)
    (DEST / "AppIcon.icns").write_bytes(b"icns" + struct.pack(">I",len(entries)+8) + entries)
