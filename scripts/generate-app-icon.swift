#!/usr/bin/swift
/// Generates COMRAD.iconset/ next to this script's output directory, then converts
/// it to COMRAD.icns using iconutil.
///
/// Usage: swift scripts/generate-app-icon.swift <output-dir>
///   output-dir — where COMRAD.icns will be written (e.g. clients/macos/Resources)

import AppKit
import Foundation

let args = CommandLine.arguments
guard args.count == 2 else {
    fputs("usage: swift generate-app-icon.swift <output-dir>\n", stderr)
    exit(1)
}
let outputDir = args[1]
let iconsetPath = "\(outputDir)/COMRAD.iconset"
let icnsPath = "\(outputDir)/COMRAD.icns"

// --- Design constants -------------------------------------------------------

// macOS rounded-rectangle corner: 22.37% of size (same as system icons)
let cornerRatio: CGFloat = 0.2237

// Background: near-black with a cool blue-grey tint
let bgColor = NSColor(red: 0.075, green: 0.090, blue: 0.118, alpha: 1)

// Subtle inner glow so it doesn't look flat on dark backgrounds
let glowColor = NSColor(red: 0.15, green: 0.25, blue: 0.45, alpha: 0.35)

// CPU symbol: white, semi-transparent so it sits softly on the dark bg
let symbolColor = NSColor.white.withAlphaComponent(0.92)

// Status dot: green (mirrors tray icon "ready" state)
let dotColor = NSColor(red: 0.20, green: 0.78, blue: 0.35, alpha: 1)   // #33C759-ish

// --- Sizes ------------------------------------------------------------------

let entries: [(filename: String, px: Int)] = [
    ("icon_16x16.png",      16),
    ("icon_16x16@2x.png",   32),
    ("icon_32x32.png",      32),
    ("icon_32x32@2x.png",   64),
    ("icon_128x128.png",    128),
    ("icon_128x128@2x.png", 256),
    ("icon_256x256.png",    256),
    ("icon_256x256@2x.png", 512),
    ("icon_512x512.png",    512),
    ("icon_512x512@2x.png", 1024),
]

// --- Render -----------------------------------------------------------------

func renderIcon(px: Int) -> NSImage {
    let s = CGFloat(px)

    return NSImage(size: NSSize(width: s, height: s), flipped: false) { rect in

        guard let ctx = NSGraphicsContext.current?.cgContext else { return false }

        // Clip to rounded rectangle
        let corner = s * cornerRatio
        let rrPath = CGPath(roundedRect: rect, cornerWidth: corner, cornerHeight: corner, transform: nil)
        ctx.saveGState()
        ctx.addPath(rrPath)
        ctx.clip()

        // Background fill
        ctx.setFillColor(bgColor.cgColor)
        ctx.fill(rect)

        // Radial glow centred slightly above-centre
        let glowRadius = s * 0.55
        let cx = s * 0.5
        let cy = s * 0.54
        let gradient = CGGradient(
            colorsSpace: CGColorSpaceCreateDeviceRGB(),
            colors: [glowColor.cgColor, glowColor.withAlphaComponent(0).cgColor] as CFArray,
            locations: [0, 1]
        )!
        ctx.drawRadialGradient(
            gradient,
            startCenter: CGPoint(x: cx, y: cy), startRadius: 0,
            endCenter:   CGPoint(x: cx, y: cy), endRadius: glowRadius,
            options: []
        )

        ctx.restoreGState()

        // CPU symbol — drawn into a square inset so it sits well in the rounded bg.
        // At very small sizes (≤32px) skip the symbol and just show dot + plain bg.
        if s >= 64 {
            let symInset = s * 0.195
            let symRect = NSRect(
                x: symInset, y: symInset,
                width: s - symInset * 2, height: s - symInset * 2
            )
            let cfg = NSImage.SymbolConfiguration(pointSize: s * 0.46, weight: .light)
                .applying(NSImage.SymbolConfiguration(paletteColors: [symbolColor]))
            if let sym = NSImage(systemSymbolName: "cpu", accessibilityDescription: nil)?
                .withSymbolConfiguration(cfg) {
                sym.draw(in: symRect, from: .zero, operation: .sourceOver, fraction: 1)
            }
        }

        // Status dot — bottom-right, same visual language as the tray icon
        let dotDiameter = s * (s >= 64 ? 0.235 : 0.42)
        let dotPad = s * 0.055
        let dotRect = NSRect(
            x: s - dotDiameter - dotPad,
            y: dotPad,
            width: dotDiameter,
            height: dotDiameter
        )

        // White ring behind the dot so it reads on any Dock background
        let ringWidth = max(1, s * 0.028)
        ctx.saveGState()
        ctx.setFillColor(bgColor.cgColor)
        let ringRect = dotRect.insetBy(dx: -ringWidth, dy: -ringWidth)
        ctx.fillEllipse(in: ringRect)
        ctx.restoreGState()

        dotColor.setFill()
        NSBezierPath(ovalIn: dotRect).fill()

        return true
    }
}

// --- Write iconset ----------------------------------------------------------

let fm = FileManager.default
try! fm.createDirectory(atPath: iconsetPath, withIntermediateDirectories: true)

for entry in entries {
    let img = renderIcon(px: entry.px)
    guard let cgImg = img.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
        fputs("error: could not get CGImage for \(entry.filename)\n", stderr)
        exit(1)
    }
    let rep = NSBitmapImageRep(cgImage: cgImg)
    rep.size = NSSize(width: entry.px, height: entry.px)
    guard let png = rep.representation(using: .png, properties: [:]) else {
        fputs("error: PNG encoding failed for \(entry.filename)\n", stderr)
        exit(1)
    }
    let dest = "\(iconsetPath)/\(entry.filename)"
    try! png.write(to: URL(fileURLWithPath: dest))
    print("  \(entry.filename)  [\(entry.px)×\(entry.px)]")
}

print("==> iconset written to \(iconsetPath)")

// --- Convert to .icns -------------------------------------------------------

let result = Process()
result.executableURL = URL(fileURLWithPath: "/usr/bin/iconutil")
result.arguments = ["-c", "icns", "-o", icnsPath, iconsetPath]
try! result.run()
result.waitUntilExit()

if result.terminationStatus == 0 {
    try? fm.removeItem(atPath: iconsetPath)
    print("==> COMRAD.icns written to \(icnsPath)")
} else {
    fputs("error: iconutil failed\n", stderr)
    exit(1)
}
