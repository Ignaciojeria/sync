---
version: alpha
name: Sunset
description: Tema cálido, editorial y con contraste suave.
colors:
  primary: "#c2410c"
  secondary: "#9a3412"
  tertiary: "#f59e0b"
  neutral: "#fff7ed"
  surface: "#ffedd5"
  on-surface: "#431407"
  info: "#0284c7"
  success: "#15803d"
  warning: "#d97706"
  error: "#b91c1c"
typography:
  body-md:
    fontFamily: "Public Sans, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.6
  label-md:
    fontFamily: "Public Sans, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 600
  code-md:
    fontFamily: "JetBrains Mono, monospace"
    fontSize: "14px"
rounded:
  sm: "0.75rem"
  md: "1.25rem"
spacing:
  xs: "0.25rem"
  sm: "0.75rem"
  md: "1rem"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#ffffff"
    rounded: "{rounded.sm}"
    padding: "{spacing.sm}"
x-pi:
  themeId: sunset
  colorScheme: light
  daisyui:
    primary-content: "#ffffff"
    accent: "{colors.tertiary}"
    base-100: "{colors.neutral}"
    base-200: "{colors.surface}"
    base-content: "{colors.on-surface}"
    neutral: "#7c2d12"
    neutral-content: "#fff7ed"
    radius-box: "{rounded.md}"
    radius-field: "{rounded.sm}"
    radius-selector: "{rounded.sm}"
    shadow-card: "0 22px 52px -28px color-mix(in srgb, #7c2d12 20%, transparent)"
    shadow-card-soft: "0 12px 24px -18px color-mix(in srgb, #7c2d12 12%, transparent)"
    shadow-sidebar: "0 24px 52px -30px color-mix(in srgb, #7c2d12 24%, transparent)"
    border-subtle: "color-mix(in srgb, #431407 12%, transparent)"
    surface-muted: "color-mix(in srgb, #ffedd5 82%, white)"
    surface-elevated: "color-mix(in srgb, #fff7ed 96%, white)"
---

# Sunset

## Overview

Tema cálido y editorial, pensado para superficies suaves y jerarquía amable.

## Colors

La paleta usa naranjas profundos y fondos crema para aportar personalidad sin
perder legibilidad.

## Typography

Public Sans domina la interfaz para mantener claridad y un tono contemporáneo.
