---
version: alpha
name: Forest
description: Tema oscuro, técnico y enfocado en consola.
colors:
  primary: "#22c55e"
  secondary: "#0f766e"
  tertiary: "#84cc16"
  neutral: "#0b1220"
  surface: "#111827"
  on-surface: "#e5f7ec"
  info: "#38bdf8"
  success: "#22c55e"
  warning: "#f59e0b"
  error: "#f87171"
typography:
  body-md:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.5
  label-md:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 600
  code-md:
    fontFamily: "JetBrains Mono, monospace"
    fontSize: "14px"
rounded:
  sm: "0.375rem"
  md: "0.75rem"
spacing:
  xs: "0.25rem"
  sm: "0.5rem"
  md: "1rem"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#052e16"
    rounded: "{rounded.sm}"
    padding: "{spacing.sm}"
x-pi:
  themeId: forest
  colorScheme: dark
  daisyui:
    primary-content: "#052e16"
    accent: "{colors.tertiary}"
    base-100: "{colors.neutral}"
    base-200: "{colors.surface}"
    base-300: "#1f2937"
    base-content: "{colors.on-surface}"
    neutral: "#030712"
    neutral-content: "#e5f7ec"
    radius-box: "{rounded.md}"
    radius-field: "{rounded.sm}"
    radius-selector: "{rounded.sm}"
    shadow-card: "0 20px 50px -28px color-mix(in srgb, #020617 52%, transparent)"
    shadow-card-soft: "0 10px 24px -18px color-mix(in srgb, #020617 34%, transparent)"
    shadow-sidebar: "0 24px 54px -30px color-mix(in srgb, #020617 58%, transparent)"
    border-subtle: "color-mix(in srgb, #e5f7ec 10%, transparent)"
    surface-muted: "color-mix(in srgb, #111827 88%, #0b1220)"
    surface-elevated: "color-mix(in srgb, #111827 72%, #1f2937)"
---

# Forest

## Overview

Tema oscuro de carácter técnico, ideal para flujos con consola, métricas y alta
concentración visual.

## Colors

Verdes fríos y fondos profundos construyen contraste sin llegar al negro puro.

## Typography

Inter mantiene la lectura general y JetBrains Mono acentúa el tono operativo.
