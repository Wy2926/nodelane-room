"""Render NSIS bitmap artwork from the existing NodeLane icon geometry (stdlib)."""
import math
from pathlib import Path
import struct


def render(destination):
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    for name, width, height, scale, left, top in (
        ('welcome', 164, 314, 0.85, 28, 56),
        ('header', 150, 57, 0.34, 99, 7),
    ):
        stride = (width * 3 + 3) & ~3
        pixels = bytearray(stride * height)
        for y in range(height):
            for x in range(width):
                if name == 'header':
                    color = (255, 255, 255)
                else:
                    glow = max(0, 1 - math.hypot((x - 120) / 230, (y - 80) / 330))
                    color = (int(9 + 9 * glow), int(19 + 26 * glow), int(34 + 28 * glow))
                    # Fine orbital arcs keep the installer in the client's midnight palette.
                    radius = math.hypot(x - 200, y - 265)
                    if any(abs(radius - r) < 0.65 for r in (115, 140, 165)):
                        color = (35, 66, 86)
                u, v = (x - left) / scale, (y - top) / scale
                if 0 <= u <= 128 and 0 <= v <= 128:
                    dx, dy = max(28 - u, 0, u - 100), max(28 - v, 0, v - 100)
                    if dx * dx + dy * dy <= 28 * 28:
                        color = (17, 42, 53)
                    for ax, ay, bx, by in ((31, 92, 31, 36), (31, 36, 44, 36), (44, 36, 84, 92), (84, 92, 97, 92), (97, 92, 97, 36)):
                        t = max(0, min(1, ((u - ax) * (bx - ax) + (v - ay) * (by - ay)) / ((bx - ax) ** 2 + (by - ay) ** 2)))
                        if math.hypot(u - ax - t * (bx - ax), v - ay - t * (by - ay)) <= 6:
                            color = (121, 225, 196)
                    if min(math.hypot(u - 31, v - 36), math.hypot(u - 97, v - 92)) <= 9:
                        color = (244, 249, 247)
                offset = (height - 1 - y) * stride + x * 3
                pixels[offset:offset + 3] = bytes(reversed(color))
        header = struct.pack('<2sIHHI', b'BM', 54 + len(pixels), 0, 0, 54)
        header += struct.pack('<IiiHHIIiiII', 40, width, height, 1, 24, 0, len(pixels), 2835, 2835, 0, 0)
        (destination / (name + '.bmp')).write_bytes(header + pixels)
