#!/usr/bin/env python3
"""Render verified synthetic demo evidence. Requires Pillow 12.3.0.

Run `make demo` first. Images are always written inside output-images/.
This renders a report from actual data; it is not a fabricated terminal capture.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
import textwrap

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[1]
REPORT = ROOT / "demo-output" / "report.json"
OUTPUT = ROOT / "output-images" / "final-output.png"


def font(size: int, mono: bool = False):
    paths = (
        ["/System/Library/Fonts/Menlo.ttc", "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"]
        if mono else
        ["/System/Library/Fonts/Supplemental/Arial.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"]
    )
    for path in paths:
        if Path(path).exists():
            return ImageFont.truetype(path, size)
    return ImageFont.load_default(size=size)


def all_values(value):
    yield value
    if isinstance(value, dict):
        for child in value.values():
            yield from all_values(child)
    elif isinstance(value, list):
        for child in value:
            yield from all_values(child)


def attr(owner, key):
    for item in owner.get("attributes", []):
        if item["key"] == key:
            value = item["value"]
            for kind in ("stringValue", "intValue", "boolValue"):
                if kind in value:
                    return str(value[kind])
            return "[structured value]"
    return "[removed]"


def span_list(trace):
    return [span for resource in trace["resourceSpans"] for scope in resource["scopeSpans"] for span in scope["spans"]]


def main():
    source = REPORT.read_bytes()
    report = json.loads(source)
    before, after = report["before"], report["after"]
    checks = report["checks"]
    if len(checks) != 7 or not span_list(after):
        raise ValueError("Expected a complete verified demo report; run make demo")
    sensitive = {"authorization", "api.key", "password"}
    key_count = lambda doc: sum(isinstance(v, dict) and v.get("key") in sensitive for v in all_values(doc))
    removed = key_count(before) - key_count(after)
    redacted = sum(v == "[REDACTED]" for v in all_values(after) if isinstance(v, str))
    first_before, first_after = span_list(before)[0], span_list(after)[0]

    im = Image.new("RGB", (1400, 1230), "#0a1220")
    d = ImageDraw.Draw(im)
    white, muted, teal, amber = "#eef4fb", "#a5b7cc", "#5ee1bd", "#ffca83"
    d.text((64, 42), "OTEL / TRACE PIPELINE", font=font(18, True), fill=teal)
    d.text((60, 77), "otel-redactor", font=font(62), fill=white)
    d.text((64, 156), "Verified local OTLP round trip  /  synthetic data only", font=font(24), fill=muted)

    for x, number, label in [(64, str(removed), "secret attributes removed"), (498, str(redacted), "string values redacted"), (932, str(len(span_list(after))), "spans preserved")]:
        d.rounded_rectangle((x, 214, x + 404, 352), radius=18, fill="#142438", outline="#2a3d53")
        d.text((x + 24, 229), number, font=font(54), fill=teal)
        d.text((x + 24, 303), label, font=font(20), fill=muted)

    d.rounded_rectangle((64, 382, 1336, 456), radius=14, fill="#102c30")
    d.text((98, 405), "OTLP HTTP input  ->  otelredactor  ->  OTLP HTTP output", font=font(25, True), fill=teal)

    d.text((64, 487), "SELECTED FIELDS", font=font(18, True), fill=muted)
    d.text((550, 487), "BEFORE", font=font(18, True), fill=amber)
    d.text((1020, 487), "AFTER", font=font(18, True), fill=teal)
    rows = [
        ("span.password", attr(first_before, "password"), attr(first_after, "password")),
        ("span.user.email", attr(first_before, "user.email"), attr(first_after, "user.email")),
        ("event.name", first_before["events"][0]["name"], first_after["events"][0]["name"]),
        ("http.response.status_code", attr(first_before, "http.response.status_code"), attr(first_after, "http.response.status_code")),
        ("child span name", span_list(before)[1]["name"], span_list(after)[1]["name"]),
    ]
    for index, (label, old, new) in enumerate(rows):
        y = 528 + index * 54
        d.rounded_rectangle((64, y - 8, 1336, y + 38), radius=8, fill="#111e30" if index % 2 == 0 else "#0e1929")
        d.text((82, y), label, font=font(19, True), fill=muted)
        d.text((550, y), old, font=font(19, True), fill=amber if old != new else white)
        d.text((1020, y), new, font=font(19, True), fill=teal if old != new else white)

    d.text((64, 822), "VERIFICATION", font=font(18, True), fill=muted)
    y = 862
    for check in checks:
        d.text((64, y), "PASS", font=font(18, True), fill=teal)
        for line in textwrap.wrap(check, width=105):
            d.text((136, y), line, font=font(19), fill=white)
            y += 25
        y += 10

    d.line((64, 1151, 1336, 1151), fill="#2a3d53", width=1)
    digest = hashlib.sha256(source).hexdigest()[:16]
    d.text((64, 1174), f"Source: demo-output/report.json  |  SHA256 {digest}  |  Run: make demo", font=font(17, True), fill=muted)
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    im.save(OUTPUT, optimize=True)
    print(OUTPUT)


if __name__ == "__main__":
    main()
